package keystorager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/lamassuiot/lamassuiot/v2/pkg/helpers"
	"github.com/sirupsen/logrus"
)

type AWSSecretsManagerKeyStorager struct {
	logger    *logrus.Entry
	smngerCli *secretsmanager.Client
}

func NewAWSSecretManagerKeyStorage(logger *logrus.Entry, awsConf aws.Config, metadata map[string]any) (KeyStorager, error) {
	log := logger.WithField("subsystem-provider", "AWS SecretsManager Client")

	httpCli, err := helpers.BuildHTTPClientWithTracerLogger(http.DefaultClient, log)
	if err != nil {
		return nil, err
	}

	awsConf.HTTPClient = httpCli

	smCli := secretsmanager.NewFromConfig(awsConf)

	return &AWSSecretsManagerKeyStorager{
		smngerCli: smCli,
		logger:    log,
	}, nil
}

func (engine *AWSSecretsManagerKeyStorager) Get(keyID string) ([]byte, error) {
	engine.logger.Debugf("Getting the key with ID: %s", keyID)

	result, err := engine.smngerCli.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(keyID),
	})
	if err != nil {
		engine.logger.Errorf("could not get Secret Value: %s", err)
		return nil, err
	}

	// Decrypts secret using the associated KMS key.
	var secretString string = *result.SecretString
	var keyMap map[string]string

	err = json.Unmarshal([]byte(secretString), &keyMap)
	if err != nil {
		return nil, err
	}

	b64Key, ok := keyMap["key"]
	if !ok {
		engine.logger.Errorf("'key' variable not found in secret")
		return nil, fmt.Errorf("'key' not found in secret")
	}

	pemBytes, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, err
	}

	return pemBytes, nil
}

func (engine *AWSSecretsManagerKeyStorager) Create(keyID string, key []byte) error {
	b64Key := base64.StdEncoding.EncodeToString(key)
	keyVal := `{"key": "` + b64Key + `"}`

	_, err := engine.smngerCli.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         aws.String(keyID),
		SecretString: aws.String(keyVal),
	})

	if err != nil {
		engine.logger.Error("Could not import RSA private key: ", err)
		return err
	}

	return nil
}

func (engine *AWSSecretsManagerKeyStorager) Delete(keyID string) error {
	return fmt.Errorf("cannot delete key [%s]. Go to your aws account and do it manually", keyID)
}
