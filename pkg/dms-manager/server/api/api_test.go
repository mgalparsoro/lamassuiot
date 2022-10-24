package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"strconv"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/lamassuiot/lamassuiot/pkg/dms-manager/common/api"
	"github.com/lamassuiot/lamassuiot/pkg/dms-manager/server/api/service"
	"github.com/lamassuiot/lamassuiot/pkg/utils"
	testUtils "github.com/lamassuiot/lamassuiot/pkg/utils/test/utils"
)

func TestCreateDMS(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name:                  "ShouldCreateDMS",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"subject":{"common_name":"My DMS Server"},"key_metadata":{"type":"RSA","bits":2048}}`
				obj := e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON()

				obj.Object().ContainsKey("dms")
				obj.Object().ContainsKey("private_key")

				dmsObj := obj.Object().Value("dms").Object()
				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate_request")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status":        "PENDING_APPROVAL",
					"serial_number": "",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})

				b64PemEncodedKey := obj.Object().Value("private_key").String().Raw()
				pemEncodedKey, err := base64.StdEncoding.DecodeString(b64PemEncodedKey)
				if err != nil {
					t.Errorf("Failed to decode b64 encoded private key: %v", err)
				}

				derKey, _ := pem.Decode(pemEncodedKey)
				rsaKey, err := x509.ParsePKCS1PrivateKey(derKey.Bytes)
				if err != nil {
					t.Errorf("Failed to parse private key: %v", err)
				}

				if rsaKey.N.BitLen() != 2048 {
					t.Errorf("Expected 2048 bit key, got %d", rsaKey.N.BitLen())
				}

				b64PemEncodedCsr := dmsObj.Value("certificate_request").String().Raw()
				pemEncodedCsr, err := base64.StdEncoding.DecodeString(b64PemEncodedCsr)
				if err != nil {
					t.Errorf("Failed to decode b64 encoded CSR: %v", err)
				}

				csrDecoded, _ := pem.Decode(pemEncodedCsr)
				csr, err := x509.ParseCertificateRequest(csrDecoded.Bytes)
				if err != nil {
					t.Errorf("Failed to parse CSR: %v", err)
				}

				if csr.Subject.CommonName != "My DMS Server" {
					t.Errorf("Expected common name to be 'My DMS Server', got %s", csr.Subject.CommonName)
				}
			},
		},
		{
			name: "ShouldNotCreateDuplicateDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"subject":{"common_name":"My DMS Server"},"key_metadata":{"type":"RSA","bits":2048}}`
				_ = e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusConflict)
			},
		},
		{
			name:                  "Validation:EmptyCommonName",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"subject":{},"key_metadata":{"type":"RSA","bits":2048}}`
				_ = e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "Validation:EmptySubject",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"key_metadata":{"type":"RSA","bits":2048}}`
				_ = e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "Validation:InvalidKeyBits",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"subject":{"common_name":"My DMS Server"},"key_metadata":{"type":"RSA","bits":2049}}`
				_ = e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "Validation:NoKeyBits",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"subject":{"common_name":"My DMS Server"}}`
				_ = e.POST("/v1/").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func TestCreateDMSWithCertificateRequest(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name:                  "ShouldCreateDMS",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				_, generatedCSR := generateBase64EncodedCertificateRequest("My DMS Server")
				reqBytes := `{"certificate_request":"` + generatedCSR + `"}`
				dmsObj := e.POST("/v1/csr").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate_request")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status":        "PENDING_APPROVAL",
					"serial_number": "",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})

				b64PemEncodedCsr := dmsObj.Value("certificate_request").String().Raw()
				if generatedCSR != b64PemEncodedCsr {
					t.Errorf("Expected CSR to be %s, got %s", generatedCSR, b64PemEncodedCsr)
				}
			},
		},
		{
			name: "ShouldNotCreateDuplicateDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				_, generatedCSR := generateBase64EncodedCertificateRequest("My DMS Server")
				reqBytes := `{"certificate_request":"` + generatedCSR + `"}`
				_ = e.POST("/v1/csr").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusConflict)
			},
		},
		{
			name:                  "Validation:EmptyCertificateRequest",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"certificate_request":""}`
				_ = e.POST("/v1/csr").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "Validation:StringNotBase64Encoded",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"certificate_request":"11236544"}`
				_ = e.POST("/v1/csr").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "Validation:StringNotPEMEncoded",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				encodedString := base64.StdEncoding.EncodeToString([]byte("this is not a PEM csr"))
				reqBytes := `{"certificate_request":"` + encodedString + `"}`
				_ = e.POST("/v1/csr").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func TestUpdateDMSStatus(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name: "ShouldUpdateFromPendingToApproved",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"APPROVED"}`
				obj := e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate")
				obj.ContainsKey("serial_number")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "APPROVED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})

				serialNumber := obj.Value("serial_number").String().Raw()
				b64PemEncodedCrt := obj.Value("certificate").String().Raw()
				pemEncodedCrt, err := base64.StdEncoding.DecodeString(b64PemEncodedCrt)
				if err != nil {
					t.Errorf("Failed to decode b64 encoded CSR: %v", err)
				}

				crtDecoded, _ := pem.Decode(pemEncodedCrt)
				crt, err := x509.ParseCertificate(crtDecoded.Bytes)
				if err != nil {
					t.Errorf("Failed to parse CRT: %v", err)
				}

				if crt.Subject.CommonName != "My DMS Server" {
					t.Errorf("Expected common name to be 'My DMS Server', got %s", crt.Subject.CommonName)
				}

				if utils.InsertNth(utils.ToHexInt(crt.SerialNumber), 2) != serialNumber {
					t.Errorf("Expected serial number to be %s, got %s", utils.InsertNth(utils.ToHexInt(crt.SerialNumber), 2), serialNumber)
				}
			},
		},
		{
			name: "ShouldUpdateFromPendingToRejected",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"REJECTED"}`
				obj := e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate_request")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"serial_number": "",
					"status":        "REJECTED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldUpdateFromApprovedToExpired",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"EXPIRED"}`
				obj := e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate")
				obj.ContainsKey("serial_number")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "EXPIRED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldUpdateFromApprovedToRevoked",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"REVOKED"}`
				obj := e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate")
				obj.ContainsKey("serial_number")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "REVOKED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldNotUpdateFromApprovedToPending",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"PENDING_APPROVAL"}`
				_ = e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name: "ShouldNotUpdateFromPendingToRevoked",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"REVOKED"}`
				_ = e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name: "ShouldNotUpdateFromPendingToExpired",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"EXPIRED"}`
				_ = e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name: "InvalidStatus",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"NEW_STATUS"}`
				_ = e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name:                  "NoDMS",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"status":"APPROVED"}`
				_ = e.PUT("/v1/My DMS Server/status").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusNotFound)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			// _, err = (*svcCA).CreateCA(context.Background(), &caApi.CreateCAInput{
			// 	CAType: caApi.CATypeDMSEnroller,
			// 	Subject: caApi.Subject{
			// 		CommonName: "LAMASSU-DMS-MANAGER",
			// 	},
			// 	KeyMetadata: caApi.KeyMetadata{
			// 		KeyType: "RSA",
			// 		KeyBits: 4096,
			// 	},
			// 	CADuration:       time.Hour * 24 * 365 * 5,
			// 	IssuanceDuration: time.Hour * 24 * 365 * 3,
			// })
			// if err != nil {
			// 	t.Errorf("%s", err)
			// }

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}

			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func TestUpdateDMSAuthorizedCAs(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name: "ShouldUpdateAuthorizedCAs",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"authorized_cas":["ca1","ca2"]}`
				obj := e.PUT("/v1/My DMS Server/auth").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate")
				obj.ContainsKey("serial_number")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{"ca1", "ca2"},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "APPROVED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldNotUpdateAuthorizedCAsInPendingStatus",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"authorized_cas":["ca1","ca2"]}`
				_ = e.PUT("/v1/My DMS Server/auth").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusBadRequest)
			},
		},
		{
			name: "ShouldRemoveAuthorizedCAs",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}

				_, err = (*svc).UpdateDMSAuthorizedCAs(context.Background(), &api.UpdateDMSAuthorizedCAsInput{
					Name:          "My DMS Server",
					AuthorizedCAs: []string{"ca1", "ca2"},
				})
				if err != nil {
					t.Errorf("Failed to update DMS authorized CAs: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				reqBytes := `{"authorized_cas":[]}`
				obj := e.PUT("/v1/My DMS Server/auth").WithBytes([]byte(reqBytes)).
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("last_status_update_timestamp")
				obj.ContainsKey("creation_timestamp")
				obj.ContainsKey("certificate")
				obj.ContainsKey("serial_number")

				obj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "APPROVED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			/*_, err = (*svcCA).CreateCA(context.Background(), &caApi.CreateCAInput{
				CAType: caApi.CATypeDMSEnroller,
				Subject: caApi.Subject{
					CommonName: "LAMASSU-DMS-MANAGER",
				},
				KeyMetadata: caApi.KeyMetadata{
					KeyType: "RSA",
					KeyBits: 4096,
				},
				CADuration:       time.Hour * 24 * 365 * 5,
				IssuanceDuration: time.Hour * 24 * 365 * 3,
			})
			if err != nil {
				t.Errorf("%s", err)
			}*/

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}

			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func TestGetDMS(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name: "ShouldGetPendingDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				dmsObj := e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate_request")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status":        "PENDING_APPROVAL",
					"serial_number": "",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldGetRejectedDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusRejected,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				dmsObj := e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate_request")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status":        "REJECTED",
					"serial_number": "",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldGetApprovedDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				dmsObj := e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate")
				dmsObj.ContainsKey("serial_number")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "APPROVED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldGetExpiredDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusExpired,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				dmsObj := e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate")
				dmsObj.ContainsKey("serial_number")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "EXPIRED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name: "ShouldGetRevokedDMS",
			serviceInitialization: func(svc *service.Service) {
				_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
					Subject: api.Subject{
						CommonName: "My DMS Server",
					},
					KeyMetadata: api.KeyMetadata{
						KeyType: "RSA",
						KeyBits: 2048,
					},
				})
				if err != nil {
					t.Errorf("Failed to create DMS: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusApproved,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}

				_, err = (*svc).UpdateDMSStatus(context.Background(), &api.UpdateDMSStatusInput{
					Name:   "My DMS Server",
					Status: api.DMSStatusRevoked,
				})
				if err != nil {
					t.Errorf("Failed to update DMS status: %v", err)
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				dmsObj := e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusOK).JSON().Object()

				dmsObj.ContainsKey("last_status_update_timestamp")
				dmsObj.ContainsKey("creation_timestamp")
				dmsObj.ContainsKey("certificate")
				dmsObj.ContainsKey("serial_number")

				dmsObj.ContainsMap(map[string]interface{}{
					"authorized_cas": []string{},
					"name":           "My DMS Server",
					"key_metadata": map[string]interface{}{
						"bits":     2048,
						"strength": "MEDIUM",
						"type":     "RSA",
					},
					"status": "REVOKED",
					"subject": map[string]interface{}{
						"common_name":       "My DMS Server",
						"country":           "",
						"locality":          "",
						"organization":      "",
						"organization_unit": "",
						"state":             "",
					},
				})
			},
		},
		{
			name:                  "NoDMS",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				e.GET("/v1/My DMS Server").
					Expect().
					Status(http.StatusNotFound)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			/*_, err = (*svcCA).CreateCA(context.Background(), &caApi.CreateCAInput{
				CAType: caApi.CATypeDMSEnroller,
				Subject: caApi.Subject{
					CommonName: "LAMASSU-DMS-MANAGER",
				},
				KeyMetadata: caApi.KeyMetadata{
					KeyType: "RSA",
					KeyBits: 4096,
				},
				CADuration:       time.Hour * 24 * 365 * 5,
				IssuanceDuration: time.Hour * 24 * 365 * 3,
			})
			if err != nil {
				t.Errorf("%s", err)
			}*/

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}

			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func TestGetDMSs(t *testing.T) {
	tt := []struct {
		name                  string
		serviceInitialization func(svc *service.Service)
		testRestEndpoint      func(e *httpexpect.Expect)
	}{
		{
			name:                  "NoDMS",
			serviceInitialization: func(svc *service.Service) {},
			testRestEndpoint: func(e *httpexpect.Expect) {
				obj := e.GET("/v1/").
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("total_dmss").ValueEqual("total_dmss", 0)
				obj.ContainsKey("dmss")

				obj.Value("dmss").Array().Empty()
			},
		},
		{
			name: "ShouldReturn3OutOf10DMSs",
			serviceInitialization: func(svc *service.Service) {
				for i := 0; i < 10; i++ {
					_, err := (*svc).CreateDMS(context.Background(), &api.CreateDMSInput{
						Subject: api.Subject{
							CommonName: "My DMS Server " + strconv.Itoa(i),
						},
						KeyMetadata: api.KeyMetadata{
							KeyType: "RSA",
							KeyBits: 2048,
						},
					})
					if err != nil {
						t.Errorf("Failed to create DMS: %v", err)
					}
				}
			},
			testRestEndpoint: func(e *httpexpect.Expect) {
				obj := e.GET("/v1/").WithQuery("limit", 3).WithQuery("offset", 0).WithQuery("sort_by", "name.asc").
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("total_dmss").ValueEqual("total_dmss", 10)
				obj.ContainsKey("dmss")

				obj.Value("dmss").Array().Length().Equal(3)

				dmsIter := obj.Value("dmss").Array().Iter()
				for idx, v := range dmsIter {
					dmsName := v.Object().Value("name").String().Raw()
					if dmsName != "My DMS Server "+strconv.Itoa(idx) {
						t.Errorf("Expected DMS name to be My DMS Server %d, got %s", idx, dmsName)
					}
				}

				obj = e.GET("/v1/").WithQuery("limit", 3).WithQuery("offset", 3).WithQuery("sort_by", "name.asc").
					Expect().
					Status(http.StatusOK).JSON().Object()

				obj.ContainsKey("total_dmss").ValueEqual("total_dmss", 10)
				obj.ContainsKey("dmss")

				obj.Value("dmss").Array().Length().Equal(3)

				dmsIter = obj.Value("dmss").Array().Iter()
				for idx, v := range dmsIter {
					dmsName := v.Object().Value("name").String().Raw()
					if dmsName != "My DMS Server "+strconv.Itoa(idx+3) {
						t.Errorf("Expected DMS name to be My DMS Server %d, got %s", idx+3, dmsName)
					}
				}
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			serverCA, _, err := testUtils.BuildCATestServer()
			//cli, err := testUtils.NewVaultSecretsMock(t)
			//if err != nil {
			//	t.Errorf("%s", err)
			//}
			//server, svc, err := testUtils.BuildCATestServerWithVault(cli)

			if err != nil {
				t.Errorf("%s", err)
			}
			defer serverCA.Close()
			serverCA.Start()

			/*_, err = (*svcCA).CreateCA(context.Background(), &caApi.CreateCAInput{
				CAType: caApi.CATypeDMSEnroller,
				Subject: caApi.Subject{
					CommonName: "LAMASSU-DMS-MANAGER",
				},
				KeyMetadata: caApi.KeyMetadata{
					KeyType: "RSA",
					KeyBits: 4096,
				},
				CADuration:       time.Hour * 24 * 365 * 5,
				IssuanceDuration: time.Hour * 24 * 365 * 3,
			})
			if err != nil {
				t.Errorf("%s", err)
			}*/

			serverDMS, svcDMS, err := testUtils.BuildDMSManagerTestServer(serverCA)
			if err != nil {
				t.Errorf("%s", err)
			}

			defer serverDMS.Close()
			serverDMS.Start()

			tc.serviceInitialization(svcDMS)
			e := httpexpect.New(t, serverDMS.URL)
			tc.testRestEndpoint(e)
		})
	}
}

func generateBase64EncodedCertificateRequest(commonName string) (*rsa.PrivateKey, string) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, key)
	if err != nil {
		panic(err)
	}

	pemEncodedBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})
	csrBase64 := base64.StdEncoding.EncodeToString(pemEncodedBytes)
	return key, csrBase64
}
