package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fil-forge/guppy/cmd/blob"
	"github.com/fil-forge/guppy/cmd/unixfs"
	"github.com/fil-forge/guppy/pkg/presets"
	"github.com/fil-forge/ucantone/multikey/ed25519"
	logging "github.com/ipfs/go-log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/fil-forge/guppy/cmd/account"
	"github.com/fil-forge/guppy/cmd/delegation"
	"github.com/fil-forge/guppy/cmd/gateway"
	"github.com/fil-forge/guppy/cmd/identity"
	"github.com/fil-forge/guppy/cmd/proof"
	"github.com/fil-forge/guppy/cmd/space"
	"github.com/fil-forge/guppy/cmd/upload"
	libid "github.com/fil-forge/libforge/identity"
)

var (
	log    = logging.Logger("cmd")
	tracer = otel.Tracer("cmd")
	// path to guppy config file relative to user config directory
	configFilePath = path.Join("guppy", "config.toml")
	// path to guppy identity file relative to user config directory
	identityFilePath = path.Join("guppy", "identity.pem")
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "guppy",
	Short: "Interact with the Storacha Network",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		span := trace.SpanFromContext(cmd.Context())
		setSpanAttributes(cmd, span)
	},
	// We handle errors ourselves when they're returned from ExecuteContext.
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	cobra.OnInitialize(initConfig)
	cobra.EnableTraverseRunHooks = true
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)

	// default storacha dir: ~/.fil-forge/guppy
	homedir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get user home directory: %w", err))
	}

	rootCmd.AddCommand(unixfs.Cmd)

	rootCmd.PersistentFlags().String(
		"data-dir",
		filepath.Join(homedir, ".fil-forge/guppy"),
		"Directory containing the config and data store (default: ~/.fil-forge/guppy)",
	)
	cobra.CheckErr(viper.BindPFlag("repo.data_dir", rootCmd.PersistentFlags().Lookup("data-dir")))

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path. Attempts to load from user config directory if not set e.g. ~/.config/"+configFilePath)

	rootCmd.PersistentFlags().Bool("ui", false, "Use the guppy UI")

	// Network configuration flags
	rootCmd.PersistentFlags().StringP("network", "n", "", "Network preset name (forge, forge-test, hot, warm-staging)")
	cobra.CheckErr(viper.BindPFlag("network.name", rootCmd.PersistentFlags().Lookup("network")))

	rootCmd.PersistentFlags().String("upload-service-did", "", "Upload service DID (overrides network preset)")
	cobra.CheckErr(viper.BindPFlag("network.upload_id", rootCmd.PersistentFlags().Lookup("upload-service-did")))

	rootCmd.PersistentFlags().String("upload-service-url", "", "Upload service URL (overrides network preset)")
	cobra.CheckErr(viper.BindPFlag("network.upload_url", rootCmd.PersistentFlags().Lookup("upload-service-url")))

	rootCmd.PersistentFlags().String("receipts-url", "", "Receipts service URL (overrides network preset)")
	cobra.CheckErr(viper.BindPFlag("network.receipts_url", rootCmd.PersistentFlags().Lookup("receipts-url")))

	rootCmd.PersistentFlags().String("indexer-did", "", "Indexing service DID (overrides network preset)")
	cobra.CheckErr(viper.BindPFlag("network.indexer_id", rootCmd.PersistentFlags().Lookup("indexer-did")))

	rootCmd.PersistentFlags().String("indexer-url", "", "Indexing service URL (overrides network preset)")
	cobra.CheckErr(viper.BindPFlag("network.indexer_url", rootCmd.PersistentFlags().Lookup("indexer-url")))

	rootCmd.PersistentFlags().Bool("insecure-did-resolution", false, "Enable insecure DID resolution (overrides network preset)")
	cobra.CheckErr(rootCmd.PersistentFlags().MarkHidden("insecure-did-resolution"))
	cobra.CheckErr(viper.BindPFlag("network.insecure_did_resolution", rootCmd.PersistentFlags().Lookup("insecure-did-resolution")))

	// Preparation configuration flags
	rootCmd.PersistentFlags().Uint("replicas", presets.DefaultReplicas, "Number of replicas to request per shard")
	cobra.CheckErr(rootCmd.PersistentFlags().MarkHidden("replicas"))
	cobra.CheckErr(viper.BindPFlag("upload.replicas", rootCmd.PersistentFlags().Lookup("replicas")))

	rootCmd.PersistentFlags().String("key-file", "", "Path to a PEM file containing ed25519 private key")
	cobra.CheckErr(rootCmd.MarkPersistentFlagFilename("key-file", "pem"))
	cobra.CheckErr(viper.BindPFlag("identity.key_file", rootCmd.PersistentFlags().Lookup("key-file")))

	// Add Commands
	rootCmd.AddCommand(
		whoamiCmd,
		versionCmd,
		retrieveCmd,
		resetCmd,
		lsCmd,
		loginCmd,
		upload.Cmd,
		space.Cmd,
		proof.Cmd,
		gateway.Cmd,
		identity.Cmd,
		delegation.Cmd,
		account.Cmd,
		blob.Cmd,
	)
}

func initConfig() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("GUPPY")

	if cfgFile == "" {
		if configDir, err := os.UserConfigDir(); err == nil {
			defaultCfgFile := path.Join(configDir, configFilePath)
			if _, err := os.Stat(defaultCfgFile); err == nil {
				log.Infof("loading config automatically from: %s", defaultCfgFile)
				cfgFile = defaultCfgFile
			} else {
				// Generate a config file and identity PEM automatically if they don't
				// exist, to make onboarding easier for new users.

				// First ensure the identity file also does not exist.
				defaultIdentityFile := path.Join(configDir, identityFilePath)
				if _, err := os.Stat(defaultIdentityFile); errors.Is(err, os.ErrNotExist) {
					log.Infof("generating identity")
					signer, err := ed25519.Generate()
					if err != nil {
						log.Fatalf("failed to generate identity: %v", err)
					}
					pem, err := libid.EncodeSignerToPEM(signer)
					if err != nil {
						log.Fatalf("failed to encode identity to PEM: %v", err)
					}

					// Write the identity and config file with default values
					log.Infof("writing identity to: %s", defaultIdentityFile)
					if err := os.WriteFile(defaultIdentityFile, pem, 0600); err != nil {
						log.Fatalf("failed to write identity to file: %v", err)
					}

					// Set the value for the identity file we just created
					viper.Set("identity.key_file", defaultIdentityFile)

					log.Infof("writing config to: %s", defaultCfgFile)
					err = viper.SafeWriteConfigAs(defaultCfgFile)
					if err != nil {
						log.Fatalf("failed to write config file: %v", err)
					}
					cfgFile = defaultCfgFile
				}
			}
		}
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		cobra.CheckErr(viper.ReadInConfig())
	} else {
		// otherwise look for config.toml in current directory
		viper.SetConfigName("config")
		viper.SetConfigType("toml")
		viper.AddConfigPath(".")
		// Don't error if config file is not found - it's optional
		_ = viper.ReadInConfig()
	}
}

// ExecuteContext adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func ExecuteContext(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "cli")
	defer span.End()

	return rootCmd.ExecuteContext(ctx)
}
