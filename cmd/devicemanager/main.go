package main

import (
	"fmt"

	"github.com/lamassuiot/lamassuiot/pkg/clients"
	"github.com/lamassuiot/lamassuiot/pkg/config"
	"github.com/lamassuiot/lamassuiot/pkg/middlewares/amqppub"
	"github.com/lamassuiot/lamassuiot/pkg/models"
	"github.com/lamassuiot/lamassuiot/pkg/routes"
	"github.com/lamassuiot/lamassuiot/pkg/services"
	"github.com/lamassuiot/lamassuiot/pkg/storage/couchdb"
	log "github.com/sirupsen/logrus"
)

var (
	version   string = "v0"    // api version
	sha1ver   string = "-"     // sha1 revision used to build the program
	buildTime string = "devTS" // when the executable was built
)

func main() {
	conf, err := config.LoadConfig[config.DeviceManagerConfig]()
	if err != nil {
		log.Fatal(err)
	}

	logLevel, err := log.ParseLevel(string(conf.Logs.Level))
	if err != nil {
		log.SetLevel(log.InfoLevel)
		log.Warn("unknown log level. defaulting to 'info' log level")
	} else {
		log.SetLevel(logLevel)
	}

	_, amqpPub, err := amqppub.SetupAMQPConnection(conf.AMQPEventPublisher)
	if err != nil {
		log.Fatal(err)
	}

	devMngrStroage, err := couchdb.NewCouchDeviceManagerRepository(conf.Storage.CouchDB.HTTPConnection, conf.Storage.CouchDB.Username, conf.Storage.CouchDB.Password)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(amqpPub)

	caURL := fmt.Sprintf("%s://%s:%d", conf.CAClient.Protocol, conf.CAClient.Hostname, conf.CAClient.Port)
	caHttpClient, err := clients.BuildHTTPClient(conf.CAClient.HTTPClient)
	if err != nil {
		log.Fatal(err)
	}

	caClient := clients.NewCAClient(caHttpClient, caURL)

	dmsMngrURL := fmt.Sprintf("%s://%s:%d", conf.DMSManagerClient.Protocol, conf.DMSManagerClient.Hostname, conf.DMSManagerClient.Port)
	dmsHttpClient, err := clients.BuildHTTPClient(conf.DMSManagerClient.HTTPClient)
	if err != nil {
		log.Fatal(err)
	}
	dmsClient := clients.NewDMSManagerClient(dmsHttpClient, dmsMngrURL)

	svc := services.NewDeviceManagerService(services.ServiceDeviceManagerBuilder{
		CAClient:       caClient,
		DevicesStorage: devMngrStroage,
		DMSClient:      dmsClient,
	})

	err = routes.NewDeviceManagerHTTPLayer(svc, conf.Server, models.APIServiceInfo{
		Version:   version,
		BuildSHA:  sha1ver,
		BuildTime: buildTime,
	})
	if err != nil {
		log.Fatal(err)
	}

	forever := make(chan struct{})
	<-forever
}
