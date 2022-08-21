package transport

import (
	"context"
	"encoding/json"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/transport"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/go-kit/log"

	"github.com/gorilla/mux"
	"github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/common/api"
	"github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/server/api/endpoint"
	lamassuErrors "github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/server/api/errors"
	"github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/server/api/service"
	stdopentracing "github.com/opentracing/opentracing-go"
)

type errorer interface {
	error() error
}

func InvalidJsonFormat() error {
	return &lamassuErrors.GenericError{
		Message:    "Invalid JSON format",
		StatusCode: 400,
	}
}

func HTTPToContext(logger log.Logger) httptransport.RequestFunc {
	return func(ctx context.Context, req *http.Request) context.Context {
		// Try to join to a trace propagated in `req`.
		uberTraceId := req.Header.Values("Uber-Trace-Id")
		if uberTraceId != nil {
			logger = log.With(logger, "span_id", uberTraceId)
		} else {
			span := stdopentracing.SpanFromContext(ctx)
			logger = log.With(logger, "span_id", span)
		}
		// return context.WithValue(ctx, utils.LamassuLoggerContextKey, logger)
		return ctx
	}
}

func MakeHTTPHandler(s service.Service, logger log.Logger, otTracer stdopentracing.Tracer) http.Handler {
	r := mux.NewRouter()
	e := endpoint.MakeServerEndpoints(s, otTracer)
	options := []httptransport.ServerOption{
		httptransport.ServerBefore(HTTPToContext(logger)),
		httptransport.ServerErrorHandler(transport.NewLogErrorHandler(logger)),
		httptransport.ServerErrorEncoder(encodeError),
	}

	r.Methods("GET").Path("/health").Handler(httptransport.NewServer(
		e.HealthEndpoint,
		decodeHealthRequest,
		encodeHealthResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "Health", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("GET").Path("/connectors").Handler(httptransport.NewServer(
		e.GetCloudConnectorsEndpoint,
		decodeGetConnectorsRequest,
		enocdeGetConnectorsResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "GetCloudConnectors", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("GET").Path("/connectors/{connectorID}/devices/{deviceID}").Handler(httptransport.NewServer(
		e.GetDeviceConfigurationEndpoint,
		decodeGetDeviceConfigurationRequest,
		encodeGetDeviceConfigurationResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "GetCloudConnectorsDevices", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("POST").Path("/connectors/synchronize").Handler(httptransport.NewServer(
		e.SynchronizedCAEndpoint,
		decodeSynchronizeCARequest,
		enocdeSynchronizeCAResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "SynchronizedCA", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("PUT").Path("/connectors/{connectorID}/config").Handler(httptransport.NewServer(
		e.UpdateConnectorConfigurationEndpoint,
		decodeUpdateConnectorConfigurationRequest,
		enocdeUpdateConnectorConfigurationResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "UpdateConnectorConfiguration", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("POST").Path("/event").Handler(httptransport.NewServer(
		e.EventHandlerEndpoint,
		decodeEventHandlerRequest,
		encodeEventHandlerResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "EventHandler", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))

	r.Methods("PUT").Path("/connectors/{connectorID}/devices/{deviceID}/cert").Handler(httptransport.NewServer(
		e.UpdateDeviceCertStatusEndpoint,
		decodeUpdateDeviceCertStatusRequest,
		encodeUpdateDeviceCertStatusResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "UpdateDeviceCertStatus", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))
	r.Methods("PUT").Path("/connectors/{connectorID}/ca/{caName}").Handler(httptransport.NewServer(
		e.UpdateCAStatusEndpoint,
		decodeUpdateCAStatusRequest,
		encodeUpdateCAStatusResponse,
		append(
			options,
			httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "UpdateCaStatus", logger)),
			httptransport.ServerBefore(HTTPToContext(logger)),
		)...,
	))
	return r
}

func decodeHealthRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	return endpoint.HealthRequest{}, nil
}

func encodeHealthResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

func decodeGetConnectorsRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	return api.GetCloudConnectorsInput{}, nil
}

func enocdeGetConnectorsResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.GetCloudConnectorsOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func decodeGetDeviceConfigurationRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	connectorID := vars["connectorID"]
	deviceID := vars["deviceID"]

	return api.GetDeviceConfigurationInput{
		ConnectorID: connectorID,
		DeviceID:    deviceID,
	}, nil
}

func encodeGetDeviceConfigurationResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.GetDeviceConfigurationOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func decodeSynchronizeCARequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	type SynchronizeCAPayload struct {
		CAName      string `json:"ca_name"`
		ConnectorID string `json:"connector_id"`
	}
	var body SynchronizeCAPayload

	err = json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return nil, InvalidJsonFormat()
	}

	return api.SynchronizeCAInput{
		CAName:      body.CAName,
		ConnectorID: body.ConnectorID,
	}, nil
}

func enocdeSynchronizeCAResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.SynchronizeCAOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func decodeUpdateConnectorConfigurationRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	connectorID := vars["connectorID"]

	type UpdateCloudProviderConfigurationPayload struct {
		Config string `json:"configuration"`
	}
	var body UpdateCloudProviderConfigurationPayload

	err = json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return nil, InvalidJsonFormat()
	}

	return api.UpdateCloudProviderConfigurationInput{
		ConnectorID: connectorID,
		Config:      body.Config,
	}, nil
}

func enocdeUpdateConnectorConfigurationResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.UpdateCloudProviderConfigurationOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func decodeEventHandlerRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	var event cloudevents.Event
	json.NewDecoder(r.Body).Decode((&event))
	return event, nil
}

func encodeEventHandlerResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)

		return nil
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(response)
}

func decodeUpdateDeviceCertStatusRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	connectorID := vars["connectorID"]
	deviceID := vars["deviceID"]

	type UpdateDeviceCertificateStatusPayload struct {
		Status       string `json:"status"`
		CAName       string `json:"ca_name"`
		SerialNumber string `json:"serial_number"`
	}
	var body UpdateDeviceCertificateStatusPayload

	err = json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return nil, InvalidJsonFormat()
	}

	return api.UpdateDeviceCertificateStatusInput{
		ConnectorID:  connectorID,
		DeviceID:     deviceID,
		CAName:       body.CAName,
		SerialNumber: body.SerialNumber,
		Status:       body.Status,
	}, nil
}

func encodeUpdateDeviceCertStatusResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.UpdateDeviceCertificateStatusOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func decodeUpdateCAStatusRequest(ctx context.Context, r *http.Request) (request interface{}, err error) {
	vars := mux.Vars(r)
	CAName := vars["caName"]
	connectorID := vars["connectorID"]

	type UpdateDeviceCertificateStatusPayload struct {
		Status string `json:"status"`
	}
	var body UpdateDeviceCertificateStatusPayload

	err = json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		return nil, InvalidJsonFormat()
	}

	return api.UpdateCAStatusInput{
		CAName:      CAName,
		ConnectorID: connectorID,
		Status:      body.Status,
	}, nil
}

func encodeUpdateCAStatusResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if e, ok := response.(errorer); ok && e.error() != nil {
		encodeError(ctx, e.error(), w)
		return nil
	}

	castedResponse := response.(*api.UpdateCAStatusOutput)
	serializedResponse := castedResponse.Serialize()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(serializedResponse)
}

func encodeError(ctx context.Context, err error, w http.ResponseWriter) {
	if err == nil {
		panic("encodeError with nil error")
	}
	w.WriteHeader(codeFrom(err))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	json.NewEncoder(w).Encode(errorWrapper{Error: err.Error()})

}

type errorWrapper struct {
	Error string `json:"error"`
}

func codeFrom(err error) int {
	switch e := err.(type) {
	case *lamassuErrors.ValidationError:
		return http.StatusBadRequest
	case *lamassuErrors.DuplicateResourceError:
		return http.StatusConflict
	case *lamassuErrors.ResourceNotFoundError:
		return http.StatusNotFound
	case *lamassuErrors.GenericError:
		return e.StatusCode
	default:
		return http.StatusInternalServerError
	}
}
