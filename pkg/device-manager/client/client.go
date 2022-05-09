package lamassudevmanager

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"

	clientUtils "github.com/lamassuiot/lamassuiot/pkg/utils/client"

	"github.com/lamassuiot/lamassuiot/pkg/device-manager/common/dto"
	lamassuEstClient "github.com/lamassuiot/lamassuiot/pkg/est/client"
)

type LamassuDevManagerClient interface {
	CreateDevice(ctx context.Context, alias string, deviceID string, dmsID string, description string, tags []string, iconName string, iconColor string) (dto.Device, error)
	UpdateDeviceById(ctx context.Context, alias string, deviceID string, dmsID string, description string, tags []string, iconName string, iconColor string) (dto.Device, error)
	GetDevices(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.Device, int, error)
	GetDeviceById(ctx context.Context, deviceId string) (dto.Device, error)
	GetDevicesByDMS(ctx context.Context, dmsId string, queryParameters dto.QueryParameters) ([]dto.Device, error)
	DeleteDevice(ctx context.Context, id string) error
	RevokeDeviceCert(ctx context.Context, id string, revocationReason string) error
	GetDeviceLogs(ctx context.Context, id string) ([]dto.DeviceLog, error)
	GetDeviceCert(ctx context.Context, id string) (dto.DeviceCert, error)
	GetDeviceCertHistory(ctx context.Context, id string) ([]dto.DeviceCertHistory, error)
	GetDmsCertHistoryThirtyDays(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.DMSCertHistory, error)
	GetDmsLastIssuedCert(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.DMSLastIssued, error)

	//EST Endpoints
	CACerts(ctx context.Context, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) ([]*x509.Certificate, error)
	Enroll(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.Enroll, error)
	Reenroll(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.Enroll, error)
	ServerKeyGen(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.ServerKeyGen, error)
}

type LamassuDevManagerClientConfig struct {
	client clientUtils.BaseClient
}

func NewLamassuDevManagerClient(config clientUtils.ClientConfiguration) (LamassuDevManagerClient, error) {
	baseClient, err := clientUtils.NewBaseClient(config)
	if err != nil {
		return nil, err
	}

	return &LamassuDevManagerClientConfig{
		client: baseClient,
	}, nil
}
func (c *LamassuDevManagerClientConfig) CreateDevice(ctx context.Context, alias string, deviceID string, dmsID string, description string, tags []string, iconName string, iconColor string) (dto.Device, error) {
	body := dto.CreateDeviceRequest{
		DeviceID:    deviceID,
		Alias:       alias,
		Description: description,
		Tags:        tags,
		IconName:    iconName,
		IconColor:   iconColor,
		DmsId:       dmsID,
	}
	req, err := c.client.NewRequest("POST", "v1/devices", body)
	if err != nil {
		return dto.Device{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return dto.Device{}, err
	}
	var device dto.Device
	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &device)
	return device, nil
}
func (c *LamassuDevManagerClientConfig) UpdateDeviceById(ctx context.Context, alias string, deviceID string, dmsID string, description string, tags []string, iconName string, iconColor string) (dto.Device, error) {
	body := dto.UpdateDevicesByIdRequest{
		DeviceID:    deviceID,
		Alias:       alias,
		Description: description,
		Tags:        tags,
		IconName:    iconName,
		IconColor:   iconColor,
		DmsId:       dmsID,
	}
	req, err := c.client.NewRequest("PUT", "v1/devices/"+deviceID, body)
	if err != nil {
		return dto.Device{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return dto.Device{}, err
	}
	var device dto.Device
	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &device)
	return device, nil
}
func (c *LamassuDevManagerClientConfig) GetDevices(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.Device, int, error) {
	var newParams string
	req, err := c.client.NewRequest("GET", "v1/devices", nil)
	if err != nil {
		return []dto.Device{}, 0, err
	}
	if queryParameters.Filter != "" {
		newParams = fmt.Sprintf("filter={%s}&", queryParameters.Filter)
	} else if queryParameters.Order.Order != "" {
		newParams = fmt.Sprintf(newParams+"s={%s,%s}&", queryParameters.Order.Order, queryParameters.Order.Field)
	}

	newParams = fmt.Sprintf(newParams+"page={%d,%d}", queryParameters.Pagination.Page, queryParameters.Pagination.Offset)
	req.URL.RawQuery = newParams
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.Device{}, 0, err
	}

	var resp dto.GetDevicesResponse

	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &resp)

	var devices []dto.Device
	for _, item := range resp.Devices {
		device := dto.Device{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &device)
		devices = append(devices, device)
	}
	return devices, len(devices), nil
}
func (c *LamassuDevManagerClientConfig) GetDeviceById(ctx context.Context, deviceId string) (dto.Device, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/"+deviceId, nil)
	if err != nil {
		return dto.Device{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return dto.Device{}, err
	}
	var device dto.Device
	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &device)

	return device, nil
}
func (c *LamassuDevManagerClientConfig) GetDevicesByDMS(ctx context.Context, dmsId string, queryParameters dto.QueryParameters) ([]dto.Device, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/dms/"+dmsId, nil)
	if err != nil {
		return []dto.Device{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.Device{}, err
	}
	var resp dto.GetDevicesResponse

	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &resp)

	var devices []dto.Device
	for _, item := range resp.Devices {
		device := dto.Device{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &device)
		devices = append(devices, device)
	}
	return devices, nil
}
func (c *LamassuDevManagerClientConfig) DeleteDevice(ctx context.Context, id string) error {
	req, err := c.client.NewRequest("DELETE", "v1/devices/"+id, nil)
	if err != nil {
		return err
	}
	_, _, err = c.client.Do(req)
	if err != nil {
		return err
	}

	return nil
}
func (c *LamassuDevManagerClientConfig) RevokeDeviceCert(ctx context.Context, id string, revocationReason string) error {
	req, err := c.client.NewRequest("DELETE", "v1/devices/"+id+"/revoke", nil)
	if err != nil {
		return err
	}
	_, _, err = c.client.Do(req)
	if err != nil {
		return err
	}
	return nil
}
func (c *LamassuDevManagerClientConfig) GetDeviceLogs(ctx context.Context, id string) ([]dto.DeviceLog, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/"+id+"/logs", nil)
	if err != nil {
		return []dto.DeviceLog{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.DeviceLog{}, err
	}

	logsArrayInterface := respBody.([]interface{})
	var logs []dto.DeviceLog
	for _, item := range logsArrayInterface {
		log := dto.DeviceLog{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &log)
		logs = append(logs, log)
	}
	return logs, nil
}
func (c *LamassuDevManagerClientConfig) GetDeviceCert(ctx context.Context, id string) (dto.DeviceCert, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/"+id+"/cert", nil)
	if err != nil {
		return dto.DeviceCert{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return dto.DeviceCert{}, err
	}

	var cert dto.DeviceCert
	jsonString, _ := json.Marshal(respBody)
	json.Unmarshal(jsonString, &cert)

	return cert, nil
}
func (c *LamassuDevManagerClientConfig) GetDeviceCertHistory(ctx context.Context, id string) ([]dto.DeviceCertHistory, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/"+id+"/cert-history", nil)
	if err != nil {
		return []dto.DeviceCertHistory{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.DeviceCertHistory{}, err
	}

	logsArrayInterface := respBody.([]interface{})
	var certHistory []dto.DeviceCertHistory
	for _, item := range logsArrayInterface {
		history := dto.DeviceCertHistory{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &history)
		certHistory = append(certHistory, history)
	}
	return certHistory, nil
}
func (c *LamassuDevManagerClientConfig) GetDmsCertHistoryThirtyDays(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.DMSCertHistory, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/dms-cert-history/thirty-days", nil)
	if err != nil {
		return []dto.DMSCertHistory{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.DMSCertHistory{}, err
	}

	dmsCertHistArrayInterface := respBody.([]interface{})
	var dmsCertHistory []dto.DMSCertHistory
	for _, item := range dmsCertHistArrayInterface {
		history := dto.DMSCertHistory{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &history)
		dmsCertHistory = append(dmsCertHistory, history)
	}
	return dmsCertHistory, nil
}
func (c *LamassuDevManagerClientConfig) GetDmsLastIssuedCert(ctx context.Context, queryParameters dto.QueryParameters) ([]dto.DMSLastIssued, error) {
	req, err := c.client.NewRequest("GET", "v1/devices/dms-cert-history/last-issued", nil)
	if err != nil {
		return []dto.DMSLastIssued{}, err
	}
	respBody, _, err := c.client.Do(req)
	if err != nil {
		return []dto.DMSLastIssued{}, err
	}

	dmsLastIssuedArrayInterface := respBody.([]interface{})
	var dmsLastIssued []dto.DMSLastIssued
	for _, item := range dmsLastIssuedArrayInterface {
		lastIssued := dto.DMSLastIssued{}
		jsonString, _ := json.Marshal(item)
		json.Unmarshal(jsonString, &lastIssued)
		dmsLastIssued = append(dmsLastIssued, lastIssued)
	}
	return dmsLastIssued, nil
}
func (c *LamassuDevManagerClientConfig) CACerts(ctx context.Context, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) ([]*x509.Certificate, error) {
	estClient, err := lamassuEstClient.NewLamassuEstClient(estServerAddr, estServerCertFile, clientCertFile, clientKeyFile, nil)
	cas, err := estClient.CACerts(ctx)
	if err != nil {
		return []*x509.Certificate{}, err
	}
	return cas, nil
}
func (c *LamassuDevManagerClientConfig) Enroll(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.Enroll, error) {
	estClient, err := lamassuEstClient.NewLamassuEstClient(estServerAddr, estServerCertFile, clientCertFile, clientKeyFile, nil)
	if err != nil {
		return dto.Enroll{}, err
	}
	crt, cacrt, err := estClient.Enroll(ctx, aps, csr)
	if err != nil {
		return dto.Enroll{}, err
	}
	var enroll dto.Enroll
	enroll.Cert = crt
	enroll.CaCert = cacrt
	return enroll, nil
}
func (c *LamassuDevManagerClientConfig) Reenroll(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.Enroll, error) {
	estClient, err := lamassuEstClient.NewLamassuEstClient(estServerAddr, estServerCertFile, clientCertFile, clientKeyFile, nil)
	if err != nil {
		return dto.Enroll{}, err
	}
	crt, cacrt, err := estClient.Reenroll(ctx, csr)
	if err != nil {
		return dto.Enroll{}, err
	}
	var reenroll dto.Enroll
	reenroll.Cert = crt
	reenroll.CaCert = cacrt
	return reenroll, nil
}
func (c *LamassuDevManagerClientConfig) ServerKeyGen(ctx context.Context, csr *x509.CertificateRequest, aps string, clientCertFile string, clientKeyFile string, estServerCertFile string, estServerAddr string) (dto.ServerKeyGen, error) {
	estClient, err := lamassuEstClient.NewLamassuEstClient(estServerAddr, estServerCertFile, clientCertFile, clientKeyFile, nil)
	if err != nil {
		return dto.ServerKeyGen{}, err
	}
	crt, key, cacrt, err := estClient.ServerKeyGen(ctx, aps, csr)
	if err != nil {
		return dto.ServerKeyGen{}, err
	}
	var serverkeygen dto.ServerKeyGen
	serverkeygen.Cert = crt
	serverkeygen.CaCert = cacrt
	serverkeygen.Key = key
	return serverkeygen, nil
}
