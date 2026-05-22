package gateway

import (
	_ "embed"
	"fmt"
	"net/http"

	"github.com/fil-forge/guppy/internal/cmdutil"
	logging "github.com/ipfs/go-log/v2"
	"github.com/labstack/echo/v4"
	"github.com/mitchellh/go-wordwrap"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// port is the default port to run the gateway on.
	port = 3000
	// blockCacheCapacity defines the default number of blocks to cache in memory.
	// Blocks are typically <1MB due to IPFS chunking, so an upper bound for how
	// much memory the cache will utilize is approximately this number multiplied
	// by 1MB. e.g. capacity for 1,000 blocks ~= 1GB of memory.
	blockCacheCapacity = 1000
	// subdomainEnabled indicates whether to enable subdomain gateway mode, which
	// is disabled by default.
	subdomainEnabled = false
	// trustedEnabled indicates whether to enable trusted gateway mode, which
	// allows deserialized responses. It is enabled by default.
	trustedEnabled = true
)

//go:embed static/index.html
var indexHTML []byte

var log = logging.Logger("cmd/gateway")

func init() {
	serveCmd.Flags().IntP("block-cache-capacity", "c", blockCacheCapacity, "Number of blocks to cache in memory")
	cobra.CheckErr(viper.BindPFlag("gateway.block_cache_capacity", serveCmd.Flags().Lookup("block-cache-capacity")))

	serveCmd.Flags().IntP("port", "p", port, "Port to run the HTTP server on")
	cobra.CheckErr(viper.BindPFlag("gateway.port", serveCmd.Flags().Lookup("port")))

	serveCmd.Flags().String("advertise-url", "", wordwrap.WrapString(
		"External HTTPS URL at which this gateway is reachable by peers (e.g. "+
			"https://localhost:3443). Delegated routing responses served by the "+
			"gateway will point to this URL as the location of blocks, which must be "+
			"served over HTTPS. This option is required to enable delegated routing "+
			"responses, which are required for Kubo to retrieve content from the "+
			"gateway. If not set, the gateway will still serve content over HTTP but "+
			"will not include routing responses.",
		80))
	cobra.CheckErr(viper.BindPFlag("gateway.advertise_url", serveCmd.Flags().Lookup("advertise-url")))

	serveCmd.Flags().BoolP("subdomain", "s", subdomainEnabled, "Enabled subdomain gateway mode (e.g. <cid>.ipfs.<gateway-host>)")
	cobra.CheckErr(viper.BindPFlag("gateway.subdomain.enabled", serveCmd.Flags().Lookup("subdomain")))

	serveCmd.Flags().StringSlice("host", []string{}, "Gateway host(s) for subdomain mode (required if subdomain mode is enabled)")
	cobra.CheckErr(viper.BindPFlag("gateway.subdomain.hosts", serveCmd.Flags().Lookup("host")))

	serveCmd.Flags().BoolP("trusted", "t", trustedEnabled, "Enable trusted gateway mode (allows deserialized responses)")
	cobra.CheckErr(viper.BindPFlag("gateway.trusted", serveCmd.Flags().Lookup("trusted")))

	serveCmd.Flags().String("log-level", "", "Logging level for the gateway server (debug, info, warn, error)")
	cobra.CheckErr(viper.BindPFlag("gateway.log_level", serveCmd.Flags().Lookup("log-level")))

}

var serveCmd = &cobra.Command{
	Use:   "serve [space...]",
	Short: "Start a Storacha Network gateway",
	Long: wordwrap.WrapString(
		"Start an IPFS Gateway that operates on the Storacha Network. By default "+
			"it serves data from all authorized spaces. One or more spaces can "+
			"be specified to restrict content served to those spaces only. "+
			"Spaces can be specified by DID or by name.",
		80),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO(forrest): the gateway builds authorized-retrieval delegations with
		// go-ucanto (delegation.Delegate/FromDelegation, validator.NewProofPruner),
		// the removed client.Proofs query, and cmdutil.ResolveDIDWebAndWrap (also
		// disabled). The client was upgraded to ucantone/libforge with a different
		// delegation/proof model. Porting needs decisions on the new APIs — confirm
		// intent with Alan. Disabled until then.
		return cmdutil.NewHandledCliError(fmt.Errorf("gateway serve is temporarily disabled during the client upgrade to ucantone (TODO(forrest))"))
	},
}

func rootHandler(c echo.Context) error {
	return c.Blob(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}
