package config

type DMSconfig struct {
	BaseConfig `mapstructure:",squash"`
	Storage    PluggableStorageEngine `mapstructure:"storage"`

	CAClient struct {
		HTTPClient `mapstructure:",squash"`
	} `mapstructure:"ca_client"`

	DevManagerESTClient struct {
		HTTPClient `mapstructure:",squash"`
	} `mapstructure:"device_manager_est_client"`

	DownstreamCertificateFile string `mapstructure:"downstream_cert_file"`
}
