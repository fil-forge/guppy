package config

type IdentityConfig struct {
	// KeyFile is the path to an Ed25519 PEM key file for agent identity.
	KeyFile string `mapstructure:"key_file" validate:"required" flag:"key-file" toml:"key_file"`
}

func (i IdentityConfig) Validate() error {
	return validateConfig(i)
}
