package service

import (
	"context"
	"fmt"
	"net/url"

	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"

	consul "github.com/hashicorp/consul/api"
	lamassuCAClient "github.com/lamassuiot/lamassuiot/pkg/ca/client"
	cloudProviderClient "github.com/lamassuiot/lamassuiot/pkg/cloud-provider/client"
	cloudProvider "github.com/lamassuiot/lamassuiot/pkg/cloud-provider/common/api"
	"github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/common/api"
	cProxyErrors "github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/server/api/errors"
	"github.com/lamassuiot/lamassuiot/pkg/cloud-proxy/server/api/repository"
	clientUtils "github.com/lamassuiot/lamassuiot/pkg/utils/client"
)

type Service interface {
	Health(ctx context.Context) bool
	GetCloudConnectors(ctx context.Context, input *api.GetCloudConnectorsInput) (*api.GetCloudConnectorsOutput, error)
	GetCloudConnectorByID(ctx context.Context, input *api.GetCloudConnectorByIDInput) (*api.GetCloudConnectorByIDOutput, error)
	GetDeviceConfiguration(ctx context.Context, input *api.GetDeviceConfigurationInput) (*api.GetDeviceConfigurationOutput, error)
	SynchronizeCA(ctx context.Context, input *api.SynchronizeCAInput) (*api.SynchronizeCAOutput, error)
	UpdateCloudProviderConfiguration(ctx context.Context, input *api.UpdateCloudProviderConfigurationInput) (*api.UpdateCloudProviderConfigurationOutput, error)
	HandleCreateCAEvent(ctx context.Context, input *api.HandleCreateCAEventInput) (*api.HandleCreateCAEventOutput, error)
	HandleUpdateCAStatusEvent(ctx context.Context, input *api.HandleUpdateCAStatusEventInput) (*api.HandleUpdateCAStatusEventOutput, error)
	HandleUpdateCertificateStatusEvent(ctx context.Context, input *api.HandleUpdateCertificateStatusEventInput) (*api.HandleUpdateCertificateStatusEventOutput, error)
	HandleUpdateDMSCaCerts(ctx context.Context, input *api.HandleUpdateDMSCaCertsInput) (*api.HandleUpdateDMSCaCertsOutput, error)
	HandleForceReenrollEvent(ctx context.Context, input *api.HandleForceReenrollEventInput) (*api.HandleForceReenrollEventOutput, error)
	HandleReenrollEvent(ctx context.Context, input *api.HandleReenrollEventInput) (*api.HandleReenrollEventOutput, error)
	UpdateDeviceCertificateStatus(ctx context.Context, input *api.UpdateDeviceCertificateStatusInput) (*api.UpdateDeviceCertificateStatusOutput, error)
	UpdateCAStatus(ctx context.Context, input *api.UpdateCAStatusInput) (*api.UpdateCAStatusOutput, error)
	UpdateDeviceDigitalTwinReenrolmentStatus(ctx context.Context, input *api.UpdateDeviceDigitalTwinReenrolmentStatusInput) (*api.UpdateDeviceDigitalTwinReenrolmentStatusOutput, error)
}

type CloudProxyService struct {
	service             Service
	ConsulClient        *consul.Client
	LamassuCAClient     lamassuCAClient.LamassuCAClient
	CloudProxyDB        repository.CloudProxyRepository
	ConnectorBaseConfig clientUtils.BaseClientConfigurationuration
}

func NewCloudPorxyService(consulClient *consul.Client, cloudProxyDatabase repository.CloudProxyRepository, lamassuCAClient lamassuCAClient.LamassuCAClient, clientBaseConfig clientUtils.BaseClientConfigurationuration) Service {
	svc := CloudProxyService{
		ConsulClient:        consulClient,
		LamassuCAClient:     lamassuCAClient,
		CloudProxyDB:        cloudProxyDatabase,
		ConnectorBaseConfig: clientBaseConfig,
	}

	svc.service = &svc

	return &svc
}

func (s *CloudProxyService) SetService(svc Service) {
	s.service = svc
}

func (s *CloudProxyService) Health(ctx context.Context) bool {
	return true
}

func (cps *CloudProxyService) GetCloudConnectors(ctx context.Context, input *api.GetCloudConnectorsInput) (*api.GetCloudConnectorsOutput, error) {
	cloudConnectors := make([]api.CloudConnector, 0)
	services, _, err := cps.ConsulClient.Catalog().Service("cloud-connector", "", &consul.QueryOptions{})
	if err != nil {
		return &api.GetCloudConnectorsOutput{}, err
	}

	for _, connector := range services {
		connectorOut, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
			ConnectorID: connector.ServiceID,
		})
		if err != nil {
			continue
		}
		exist := false
		for _, cloudConnector := range cloudConnectors {
			if cloudConnector.ID == connectorOut.ID {
				exist = true
				break
			}
		}
		if !exist {
			cloudConnectors = append(cloudConnectors, connectorOut.CloudConnector)
		}
	}

	return &api.GetCloudConnectorsOutput{
		CloudConnectors: cloudConnectors,
	}, nil
}

func (cps *CloudProxyService) GetCloudConnectorByID(ctx context.Context, input *api.GetCloudConnectorByIDInput) (*api.GetCloudConnectorByIDOutput, error) {
	services, _, err := cps.ConsulClient.Catalog().Service("cloud-connector", "", &consul.QueryOptions{
		Filter: fmt.Sprintf("ServiceID == \"%s\"", input.ConnectorID),
	})

	if err != nil {
		return &api.GetCloudConnectorByIDOutput{}, err
	}

	if len(services) == 0 {
		return &api.GetCloudConnectorByIDOutput{}, &cProxyErrors.ResourceNotFoundError{ResourceType: "CloudConnector", ResourceId: input.ConnectorID}
	}
	status := "passing"
	// Get First Status == passing
	selectedServiceIdx := slices.IndexFunc(services, func(item *consul.CatalogService) bool {
		hchecks, _, err := cps.ConsulClient.Health().Node(item.Node, &consul.QueryOptions{})
		if err != nil {
			return false
		}
		serviceChecks, _, err := cps.ConsulClient.Health().Checks("cloud-connector", &consul.QueryOptions{
			Filter: fmt.Sprintf("ServiceID == \"%s\"", item.ServiceID),
		})
		if err != nil {
			return false
		}
		nodeCheck := false
		for _, check := range hchecks {
			if check.CheckID == "service:"+item.ServiceID && check.Status == "passing" {
				nodeCheck = true
			}
		}
		serviceCheck := false
		for _, sCheck := range serviceChecks {
			if sCheck.CheckID == "service:"+item.ServiceID && sCheck.Status == "passing" {
				serviceCheck = true
			}
		}
		if serviceCheck && nodeCheck {
			return true
		}

		return false
	})

	//No Status == passing service. At this point, it does not matter which one to choose. Return the first one.
	if selectedServiceIdx == -1 {
		status = "critical"
		selectedServiceIdx = 0
	}

	connector := services[selectedServiceIdx]

	connectorIp := connector.ServiceAddress
	connectorPort := connector.ServicePort
	connectorType, err := api.ParseCloudProviderType(connector.ServiceMeta["connector-type"])
	if err != nil {
		return &api.GetCloudConnectorByIDOutput{}, err
	}

	syncCAs := make([]api.SynchronizedCA, 0)
	caBindngs, err := cps.CloudProxyDB.SelectCABindingsByConnectorID(ctx, connector.ServiceID)
	if err != nil {
		return &api.GetCloudConnectorByIDOutput{}, err
	}

	for _, caBindng := range caBindngs {
		syncCAs = append(syncCAs, api.SynchronizedCA{
			CABinding:           caBindng,
			ConsistencyStatus:   api.ConsistencyStatusDisabled, //Disabled by default. If Consul Agent is available, it will be either Inconsistent or Consistent.
			CloudProviderConfig: nil,
		})
	}

	if status != "passing" {
		return &api.GetCloudConnectorByIDOutput{
			CloudConnector: api.CloudConnector{
				ID:              connector.ServiceID,
				Status:          status,
				Name:            connector.ServiceMeta["name"],
				CloudProvider:   connectorType,
				IP:              connectorIp,
				Port:            connectorPort,
				SynchronizedCAs: syncCAs,
				Protocol:        connector.ServiceMeta["protocol"],
				Configuration:   nil,
			},
		}, nil
	} else {
		connectorService, err := cps.newCloudPriverClient(connector.ServiceMeta["protocol"], connectorIp, connectorPort)
		if err != nil {
			return &api.GetCloudConnectorByIDOutput{}, err
		}

		getConfigOutput, err := connectorService.GetConfiguration(ctx, &cloudProvider.GetConfigurationInput{})
		if err != nil {
			log.Warn("msg", fmt.Sprintf("Could not get connector configuration [TYPE]=%s [ID]=%s [IP]=%s [PORT]=%d", connectorType, connector.ServiceID, connectorIp, connectorPort), "err", err)
			return &api.GetCloudConnectorByIDOutput{}, err
		}

		for idx, syncCA := range syncCAs {
			if status == "passing" {
				syncCAs[idx].ConsistencyStatus = api.ConsistencyStatusInconsistent
				for _, cloudCA := range getConfigOutput.CAsConfiguration {
					if cloudCA.CAName == syncCA.CAName {
						syncCAs[idx].ConsistencyStatus = api.ConsistencyStatusConsistent
						syncCAs[idx].CloudProviderConfig = cloudCA.Configuration
					}
				}
			}
		}

		return &api.GetCloudConnectorByIDOutput{
			CloudConnector: api.CloudConnector{
				ID:              connector.ServiceID,
				Status:          status,
				Name:            connector.ServiceMeta["name"],
				CloudProvider:   connectorType,
				IP:              connectorIp,
				Port:            connectorPort,
				Protocol:        connector.ServiceMeta["protocol"],
				SynchronizedCAs: syncCAs,
				Configuration:   getConfigOutput.Configuration,
			},
		}, nil
	}

}

func (cps *CloudProxyService) GetDeviceConfiguration(ctx context.Context, input *api.GetDeviceConfigurationInput) (*api.GetDeviceConfigurationOutput, error) {
	connector, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
		ConnectorID: input.ConnectorID,
	})
	if err != nil {
		return &api.GetDeviceConfigurationOutput{}, err
	}

	connectorClient, err := cps.newCloudPriverClientFromConnector(connector.CloudConnector)
	if err != nil {
		return &api.GetDeviceConfigurationOutput{}, err
	}

	deviceConfig, err := connectorClient.GetDeviceConfiguration(ctx, &cloudProvider.GetDeviceConfigurationInput{
		DeviceID: input.DeviceID,
	})

	if err != nil {
		return &api.GetDeviceConfigurationOutput{}, err
	}

	return &api.GetDeviceConfigurationOutput{
		Configuration: deviceConfig.Configuration,
	}, err
}

func (cps *CloudProxyService) SynchronizeCA(ctx context.Context, input *api.SynchronizeCAInput) (*api.SynchronizeCAOutput, error) {
	err := cps.CloudProxyDB.InsertCABinding(ctx, input.ConnectorID, input.CAName)
	if err != nil {
		return &api.SynchronizeCAOutput{}, err
	}

	connectorOutput, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
		ConnectorID: input.ConnectorID,
	})
	if err != nil {
		return &api.SynchronizeCAOutput{}, err
	}

	return &api.SynchronizeCAOutput{
		CloudConnector: connectorOutput.CloudConnector,
	}, nil
}

func (cps *CloudProxyService) UpdateCloudProviderConfiguration(ctx context.Context, input *api.UpdateCloudProviderConfigurationInput) (*api.UpdateCloudProviderConfigurationOutput, error) {
	connectorOutput, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
		ConnectorID: input.ConnectorID,
	})
	if err != nil {
		return &api.UpdateCloudProviderConfigurationOutput{}, err
	}

	connectorClient, err := cps.newCloudPriverClientFromConnector(connectorOutput.CloudConnector)
	if err != nil {
		return &api.UpdateCloudProviderConfigurationOutput{}, err
	}

	_, err = connectorClient.UpdateConfiguration(ctx, &cloudProvider.UpdateConfigurationInput{
		Configuration: input.Config,
	})

	return &api.UpdateCloudProviderConfigurationOutput{}, err
}

func (cps *CloudProxyService) HandleCreateCAEvent(ctx context.Context, input *api.HandleCreateCAEventInput) (*api.HandleCreateCAEventOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleCreateCAEventOutput{}, err
	}

	for _, connector := range connectorsOutput.CloudConnectors {
		for _, syncCA := range connector.SynchronizedCAs {
			if syncCA.ConsistencyStatus != api.ConsistencyStatusDisabled && syncCA.CAName == input.CAName {
				fmt.Println(fmt.Sprintf("	[%s](%s) %s  --->  %s:%d", connector.Status, connector.CloudProvider, connector.ID, connector.IP, connector.Port))
				err := cps.CloudProxyDB.UpdateCABindingSerialNumber(ctx, connector.ID, input.CAName, input.SerialNumber)
				if err != nil {
					log.Warn(err)
					continue
				}

				connectorClient, err := cps.newCloudPriverClientFromConnector(connector)
				if err != nil {
					log.Warn(err)
					continue
				}

				_, err = connectorClient.RegisterCA(ctx, &cloudProvider.RegisterCAInput{
					CACertificate: input.CACertificate,
				})
				if err != nil {
					log.Warn(err)
					continue
				}
			}
		}
	}

	return &api.HandleCreateCAEventOutput{}, nil
}

func (cps *CloudProxyService) HandleUpdateCAStatusEvent(ctx context.Context, input *api.HandleUpdateCAStatusEventInput) (*api.HandleUpdateCAStatusEventOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleUpdateCAStatusEventOutput{}, err
	}

	for _, connector := range connectorsOutput.CloudConnectors {
		for _, syncCA := range connector.SynchronizedCAs {
			if syncCA.ConsistencyStatus != api.ConsistencyStatusDisabled && syncCA.CAName == input.CAName {
				fmt.Println(fmt.Sprintf("	[%s](%s) %s  --->  %s:%d", connector.Status, connector.CloudProvider, connector.ID, connector.IP, connector.Port))
				err := cps.CloudProxyDB.UpdateCABindingSerialNumber(ctx, connector.ID, input.CAName, input.SerialNumber)
				if err != nil {
					log.Warn(err)
					continue
				}

				connectorClient, err := cps.newCloudPriverClientFromConnector(connector)
				if err != nil {
					log.Warn(err)
					continue
				}

				_, err = connectorClient.UpdateCAStatus(ctx, &cloudProvider.UpdateCAStatusInput{
					CAName: input.CAName,
					Status: string(input.Status),
				})
				if err != nil {
					log.Warn(err)
					continue
				}
			}
		}
	}
	return &api.HandleUpdateCAStatusEventOutput{}, nil
}

func (cps *CloudProxyService) HandleUpdateCertificateStatusEvent(ctx context.Context, input *api.HandleUpdateCertificateStatusEventInput) (*api.HandleUpdateCertificateStatusEventOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleUpdateCertificateStatusEventOutput{}, err
	}

	for _, connector := range connectorsOutput.CloudConnectors {
		for _, syncCA := range connector.SynchronizedCAs {
			if syncCA.ConsistencyStatus != api.ConsistencyStatusDisabled && syncCA.CAName == input.CAName {
				fmt.Println(fmt.Sprintf("	[%s](%s) %s  --->  %s:%d", connector.Status, connector.CloudProvider, connector.ID, connector.IP, connector.Port))
				err := cps.CloudProxyDB.UpdateCABindingSerialNumber(ctx, connector.ID, input.CAName, input.SerialNumber)
				if err != nil {
					log.Warn(err)
					continue
				}

				cps.service.UpdateDeviceCertificateStatus(ctx, &api.UpdateDeviceCertificateStatusInput{
					ConnectorID:  connector.ID,
					DeviceID:     input.Certificate.Subject.CommonName,
					CAName:       input.CAName,
					Status:       string(input.Status),
					SerialNumber: input.SerialNumber,
				})

				if err != nil {
					log.Warn(err)
					continue
				}
			}
		}
	}
	return &api.HandleUpdateCertificateStatusEventOutput{}, nil
}

func (cps *CloudProxyService) HandleReenrollEvent(ctx context.Context, input *api.HandleReenrollEventInput) (*api.HandleReenrollEventOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleReenrollEventOutput{}, err
	}

	for _, connector := range connectorsOutput.CloudConnectors {
		for _, syncCA := range connector.SynchronizedCAs {
			if syncCA.ConsistencyStatus != api.ConsistencyStatusDisabled && syncCA.CAName == input.Certificate.Issuer.CommonName {
				fmt.Println(fmt.Sprintf("	[%s](%s) %s  --->  %s:%d", connector.Status, connector.CloudProvider, connector.ID, connector.IP, connector.Port))
				cps.service.UpdateDeviceDigitalTwinReenrolmentStatus(ctx, &api.UpdateDeviceDigitalTwinReenrolmentStatusInput{
					ConnectorID:   connector.ID,
					DeviceID:      input.Certificate.Subject.CommonName,
					SlotID:        "",
					ForceReenroll: true,
				})

				if err != nil {
					log.Warn(err)
					continue
				}
			}
		}
	}
	return &api.HandleReenrollEventOutput{}, nil
}

func (cps *CloudProxyService) HandleUpdateDMSCaCerts(ctx context.Context, input *api.HandleUpdateDMSCaCertsInput) (*api.HandleUpdateDMSCaCertsOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleUpdateDMSCaCertsOutput{}, err
	}
	for _, connector := range connectorsOutput.CloudConnectors {
		if connectorsOutput.CloudConnectors[0].CloudProvider == api.CloudProviderAmazonWebServices {
			connectorClient, err := cps.newCloudPriverClientFromConnector(connector)
			if err != nil {
				log.Warn(err)
				continue
			}
			_, err = connectorClient.UpdateDMSCaCerts(ctx, &cloudProvider.UpdateDMSCaCertsInput{
				DeviceManufacturingService: input.DeviceManufacturingService,
			})
			if err != nil {
				log.Warn(err)
				continue
			}
		}
	}

	return &api.HandleUpdateDMSCaCertsOutput{}, nil
}

func (cps *CloudProxyService) UpdateDeviceCertificateStatus(ctx context.Context, input *api.UpdateDeviceCertificateStatusInput) (*api.UpdateDeviceCertificateStatusOutput, error) {
	connectorOutput, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
		ConnectorID: input.ConnectorID,
	})
	if err != nil {
		return &api.UpdateDeviceCertificateStatusOutput{}, err
	}

	connectorClient, err := cps.newCloudPriverClientFromConnector(connectorOutput.CloudConnector)
	if err != nil {
		return &api.UpdateDeviceCertificateStatusOutput{}, err
	}

	_, err = connectorClient.UpdateDeviceCertificateStatus(ctx, &cloudProvider.UpdateDeviceCertificateStatusInput{
		DeviceID:     input.DeviceID,
		Status:       input.Status,
		CAName:       input.CAName,
		SerialNumber: input.SerialNumber,
	})
	if err != nil {
		return &api.UpdateDeviceCertificateStatusOutput{}, err
	}

	return &api.UpdateDeviceCertificateStatusOutput{}, nil
}

func (cps *CloudProxyService) UpdateDeviceDigitalTwinReenrolmentStatus(ctx context.Context, input *api.UpdateDeviceDigitalTwinReenrolmentStatusInput) (*api.UpdateDeviceDigitalTwinReenrolmentStatusOutput, error) {
	connectorOutput, err := cps.service.GetCloudConnectorByID(ctx, &api.GetCloudConnectorByIDInput{
		ConnectorID: input.ConnectorID,
	})
	if err != nil {
		return &api.UpdateDeviceDigitalTwinReenrolmentStatusOutput{}, err
	}

	connectorClient, err := cps.newCloudPriverClientFromConnector(connectorOutput.CloudConnector)
	if err != nil {
		return &api.UpdateDeviceDigitalTwinReenrolmentStatusOutput{}, err
	}

	_, err = connectorClient.UpdateDeviceDigitalTwinStatus(ctx, &cloudProvider.UpdateDeviceDigitalTwinReenrollmentStatusInput{
		DeviceID:      input.DeviceID,
		SlotID:        input.SlotID,
		ForceReenroll: input.ForceReenroll,
	})
	if err != nil {
		return &api.UpdateDeviceDigitalTwinReenrolmentStatusOutput{}, err
	}

	return &api.UpdateDeviceDigitalTwinReenrolmentStatusOutput{}, nil
}

func (cps *CloudProxyService) UpdateCAStatus(ctx context.Context, input *api.UpdateCAStatusInput) (*api.UpdateCAStatusOutput, error) {
	return &api.UpdateCAStatusOutput{}, nil
}

func (cps *CloudProxyService) newCloudPriverClientFromConnector(connector api.CloudConnector) (cloudProviderClient.LamassuCloudProviderClient, error) {
	return cps.newCloudPriverClient(connector.Protocol, connector.IP, connector.Port)
}

func (cps *CloudProxyService) newCloudPriverClient(protocol string, ip string, port int) (cloudProviderClient.LamassuCloudProviderClient, error) {
	scheme := "http"
	if protocol == "https" {
		scheme = "https"
	}

	config := cps.ConnectorBaseConfig
	config.URL = &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", ip, port),
	}

	return cloudProviderClient.NewCloudProviderClient(config)
}
func (cps *CloudProxyService) HandleForceReenrollEvent(ctx context.Context, input *api.HandleForceReenrollEventInput) (*api.HandleForceReenrollEventOutput, error) {
	connectorsOutput, err := cps.service.GetCloudConnectors(ctx, &api.GetCloudConnectorsInput{})
	if err != nil {
		return &api.HandleForceReenrollEventOutput{}, err
	}

	for _, connector := range connectorsOutput.CloudConnectors {
		for _, syncCA := range connector.SynchronizedCAs {
			if syncCA.ConsistencyStatus != api.ConsistencyStatusDisabled && syncCA.CAName == input.Crt.Issuer.CommonName {
				fmt.Println(fmt.Sprintf("	[%s](%s) %s  --->  %s:%d", connector.Status, connector.CloudProvider, connector.ID, connector.IP, connector.Port))
				cps.service.UpdateDeviceDigitalTwinReenrolmentStatus(ctx, &api.UpdateDeviceDigitalTwinReenrolmentStatusInput{
					ConnectorID:   connector.ID,
					DeviceID:      input.Crt.Subject.CommonName,
					SlotID:        input.SlotID,
					ForceReenroll: true,
				})
				if err != nil {
					log.Warn(err)
					continue
				}
			}
		}
	}
	return &api.HandleForceReenrollEventOutput{}, nil
}