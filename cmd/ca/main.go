package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/go-openapi/runtime/middleware"

	migrate "github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/lamassuiot/lamassuiot/pkg/ca/server/api/service"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/api/transport"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/config"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/docs"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/models/ca/store"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/models/ca/store/db"
	"github.com/lamassuiot/lamassuiot/pkg/ca/server/secrets/vault"
	"github.com/lamassuiot/lamassuiot/pkg/utils"
	"github.com/opentracing/opentracing-go"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streadway/amqp"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
)

func main() {

	var logger log.Logger
	logger = log.NewJSONLogger(os.Stdout)
	logger = level.NewFilter(logger, level.AllowDebug())
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	/*********************************************************************/

	cfg, err := config.NewConfig("")
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not read environment configuration values")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Environment configuration values loaded")

	if strings.ToLower(cfg.DebugMode) == "debug" {
		{
			logger = log.NewJSONLogger(os.Stdout)
			logger = log.With(logger, "ts", log.DefaultTimestampUTC)
			logger = level.NewFilter(logger, level.AllowDebug())
			logger = log.With(logger, "caller", log.DefaultCaller)
		}
		level.Debug(logger).Log("msg", "Starting Lamassu-ca in debug mode...")
	}

	jcfg, err := jaegercfg.FromEnv()
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not load Jaeger configuration values fron environment")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Jaeger configuration values loaded")

	tracer, closer, err := jcfg.NewTracer(
		jaegercfg.Logger(jaegerlog.StdLogger),
	)
	opentracing.SetGlobalTracer(tracer)

	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not start Jaeger tracer")
		os.Exit(1)
	}
	defer closer.Close()
	level.Info(logger).Log("msg", "Jaeger tracer started")

	secretsVault, err := vault.NewVaultSecrets(cfg.VaultAddress, cfg.VaultPkiCaPath, cfg.VaultRoleID, cfg.VaultSecretID, cfg.VaultCA, cfg.VaultUnsealKeysFile, cfg.OcspUrl, logger)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not start connection with Vault Secret Engine")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Connection established with secret engine")
	casDb := initializeDB(cfg.PostgresCaDB, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHostname, cfg.PostgresPort, cfg.PostgresMigrationsFilePath, logger)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not start connection with Devices database. Will sleep for 5 seconds and exit the program")
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Connection established with Devices database")
	fieldKeys := []string{"method", "error"}

	amq_cfg := new(tls.Config)
	amq_cfg.RootCAs = x509.NewCertPool()

	if ca, err := ioutil.ReadFile(cfg.AmqpServerCACertFile); err == nil {
		amq_cfg.RootCAs.AppendCertsFromPEM(ca)
	}
	if cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err == nil {
		amq_cfg.Certificates = append(amq_cfg.Certificates, cert)
	}

	amqpConn, err := amqp.DialTLS("amqps://"+cfg.AmqpIP+":"+cfg.AmqpPort+"", amq_cfg)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Failed to connect to AMQP")
		os.Exit(1)
	}
	defer amqpConn.Close()

	amqpChannel, err := amqpConn.Channel()
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Failed to create AMQP channel")
		os.Exit(1)
	}
	defer amqpChannel.Close()

	var s service.Service
	{
		s = service.NewCAService(logger, secretsVault, casDb)
		s = service.NewAmqpMiddleware(amqpChannel, logger)(s)
		s = service.LoggingMiddleware(logger)(s)
		s = service.NewInstrumentingMiddleware(
			kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
				Namespace: "enroller",
				Subsystem: "enroller_service",
				Name:      "request_count",
				Help:      "Number of requests received.",
			}, fieldKeys),
			kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
				Namespace: "enroller",
				Subsystem: "enroller_service",
				Name:      "request_latency_microseconds",
				Help:      "Total duration of requests in microseconds.",
			}, fieldKeys),
		)(s)
	}

	openapiSpec := docs.NewOpenAPI3(cfg)

	specHandler := func(prefix string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			url := r.URL.Path
			if originalPrefix, ok := r.Header["X-Envoy-Original-Path"]; ok {
				url = originalPrefix[0]
			}
			url = strings.Split(url, prefix)[0]
			openapiSpec.Servers[0].URL = url
			openapiSpecJsonData, _ := json.Marshal(&openapiSpec)
			w.Write(openapiSpecJsonData)
		}
	}

	mux := http.NewServeMux()

	http.Handle("/v1/", accessControl(http.StripPrefix("/v1", transport.MakeHTTPHandler(s, log.With(logger, "component", "HTTPS"), tracer))))
	http.Handle("/v1/docs/", http.StripPrefix("/v1/docs", middleware.SwaggerUI(middleware.SwaggerUIOpts{
		Path:    "/",
		SpecURL: "spec.json",
	}, mux)))
	http.HandleFunc("/v1/docs/spec.json", specHandler("/v1/docs/"))
	http.Handle("/metrics", promhttp.Handler())

	errs := make(chan error)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	go func() {
		if strings.ToLower(cfg.Protocol) == "https" {
			if cfg.MutualTLSEnabled {
				mTlsCertPool, err := utils.CreateCAPool(cfg.MutualTLSClientCA)
				if err != nil {
					level.Error(logger).Log("err", err, "msg", "Could not create mTls Cert Pool")
					os.Exit(1)
				}
				tlsConfig := &tls.Config{
					ClientCAs:  mTlsCertPool,
					ClientAuth: tls.RequireAndVerifyClientCert,
				}
				tlsConfig.BuildNameToCertificate()

				http := &http.Server{
					Addr:      ":" + cfg.Port,
					TLSConfig: tlsConfig,
				}

				level.Info(logger).Log("transport", "Mutual TLS", "address", ":"+cfg.Port, "msg", "listening")
				errs <- http.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)

			} else {
				level.Info(logger).Log("transport", "HTTPS", "address", ":"+cfg.Port, "msg", "listening")
				errs <- http.ListenAndServeTLS(":"+cfg.Port, cfg.CertFile, cfg.KeyFile, nil)
			}
		} else if strings.ToLower(cfg.Protocol) == "http" {
			level.Info(logger).Log("transport", "HTTP", "address", ":"+cfg.Port, "msg", "listening")
			errs <- http.ListenAndServe(":"+cfg.Port, nil)
		} else {
			level.Error(logger).Log("err", "msg", "Unknown protocol")
			os.Exit(1)
		}
	}()

	level.Info(logger).Log("exit", <-errs)
}

func accessControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// dumpReq, err := httputil.DumpRequest(r, true)
		// if err == nil {
		// 	fmt.Println(string(dumpReq))
		// }

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			return
		}

		h.ServeHTTP(w, r)
	})
}

func initializeDB(database string, user string, password string, hostname string, port string, migrationsFilePath string, logger log.Logger) store.DB {
	casConnStr := "dbname=" + database + " user=" + user + " password=" + password + " host=" + hostname + " port=" + port + " sslmode=disable"
	casStore, err := db.NewDB("postgres", casConnStr, logger)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not start connection with database. Will sleep for 5 seconds and exit the program")
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}

	level.Info(logger).Log("msg", "Connection established with Devices database")

	level.Info(logger).Log("msg", "Checking if DB migration is required")

	devicesDb := casStore.(*db.DB)
	driver, err := migratePostgres.WithInstance(devicesDb.DB, &migratePostgres.Config{})
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not create postgres migration driver")
		os.Exit(1)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsFilePath,
		"postgres", driver)
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not create db migration instance ")
		os.Exit(1)
	}

	m.Up()
	if err != nil {
		level.Error(logger).Log("err", err, "msg", "Could not perform db migration")
		os.Exit(1)
	}
	return casStore
}
