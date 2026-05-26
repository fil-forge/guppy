package cmd

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	indexing_service "github.com/fil-forge/indexing-service/pkg/client"
	"github.com/fil-forge/libforge/bytemap"
	contentcmds "github.com/fil-forge/libforge/commands/content"
	"github.com/fil-forge/libforge/digestutil"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multihash"
	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/cmdutil"
	"github.com/fil-forge/guppy/pkg/config"
	"github.com/fil-forge/guppy/pkg/verification"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <root-cid>",
	Short: "Verify a DAG",
	Long:  `Verify the integrity and correctness of a Directed Acyclic Graph (DAG).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logging.SetLogLevel("cmd", "INFO")

		cfg, err := config.Load[config.Config]()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		root, err := cid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("parsing root CID: %w", err)
		}

		network := cmdutil.MustGetNetworkConfig(cfg.Network, "")
		guppy := cmdutil.MustGetClientForNetwork(cfg, "")

		// The spaces the agent can act on are the ones it can authorize retrievals for.
		spaces, err := guppy.Spaces()
		if err != nil {
			return fmt.Errorf("listing spaces: %w", err)
		}
		authdSpaces := make([]did.DID, 0, len(spaces))
		for _, s := range spaces {
			authdSpaces = append(authdSpaces, s.DID())
		}

		indexerClient, err := indexing_service.New(network.IndexerID, network.IndexerURL)
		cobra.CheckErr(err)

		var authorizeIndexer verification.AuthorizeIndexerRetrievalFunc
		var getProofs verification.ContentRetrieveProofGetterFunc
		if network.AuthorizedRetrievals {
			authorizeIndexer = func() ([]ucan.Delegation, error) {
				dels := make([]ucan.Delegation, 0, len(authdSpaces))
				for _, space := range authdSpaces {
					d, err := contentcmds.Retrieve.Delegate(guppy.Issuer(), network.IndexerID, space)
					if err != nil {
						return nil, err
					}
					dels = append(dels, d)
				}
				return dels, nil
			}
			getProofs = func(space did.DID) ([]ucan.Delegation, error) {
				proofs, _, err := guppy.ProofChain(cmd.Context(), guppy.Issuer().DID(), contentcmds.Retrieve.Command, space)
				return proofs, err
			}
		}

		indexer := verification.NewIndexer(indexerClient, authorizeIndexer)

		p := tea.NewProgram(newVerifyModel(root))

		var verifyErr error
		go func() {
			for msg, err := range verification.VerifyDAGRetrieval(cmd.Context(), guppy.Issuer(), getProofs, indexer, root) {
				if err != nil {
					verifyErr = err
					break
				}
				p.Send(msg)
			}
			p.Quit()
		}()

		if _, err := p.Run(); err != nil {
			return err
		}
		return verifyErr
	},
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

type verifyModel struct {
	root         cid.Cid
	blocks       bytemap.ByteMap[multihash.Multihash, struct{}] // not validated
	vblocks      bytemap.ByteMap[multihash.Multihash, struct{}] // validated
	shards       bytemap.ByteMap[multihash.Multihash, uint64]   // shard digest -> verified blocks
	size         uint64
	origins      map[did.DID]string // node DID -> hostname
	originBlocks map[did.DID]uint64 // node DID -> verified blocks
}

func newVerifyModel(root cid.Cid) verifyModel {
	blocks := bytemap.NewByteMap[multihash.Multihash, struct{}](1)
	blocks.Set(root.Hash(), struct{}{})
	return verifyModel{
		root:         root,
		blocks:       blocks,
		vblocks:      bytemap.NewByteMap[multihash.Multihash, struct{}](0),
		shards:       bytemap.NewByteMap[multihash.Multihash, uint64](0),
		origins:      map[did.DID]string{},
		originBlocks: map[did.DID]uint64{},
	}
}

func (m verifyModel) Init() tea.Cmd {
	return nil
}

func (m verifyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case verification.VerifiedBlock:
		if !m.vblocks.Has(msg.Stat.Digest) {
			m.vblocks.Set(msg.Stat.Digest, struct{}{})
			m.blocks.Delete(msg.Stat.Digest)
			m.size += msg.Stat.Size
		}
		for _, link := range msg.Stat.Links {
			if !m.vblocks.Has(link.Hash()) {
				m.blocks.Set(link.Hash(), struct{}{})
			}
		}
		shardBlockCount := m.shards.Get(msg.Stat.Origin.Shard) + 1
		m.shards.Set(msg.Stat.Origin.Shard, shardBlockCount)
		m.origins[msg.Stat.Origin.Node] = msg.Stat.Origin.URL.Hostname()
		m.originBlocks[msg.Stat.Origin.Node] = m.originBlocks[msg.Stat.Origin.Node] + 1
		return m, nil
	default:
		return m, nil
	}
}

var heading = lipgloss.NewStyle().Bold(true)
var faint = lipgloss.NewStyle().Faint(true)

func (m verifyModel) View() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(heading.Render("Root"))
	sb.WriteString("\n  ")
	sb.WriteString(m.root.String())
	sb.WriteString("\n")

	if m.shards.Size() > 0 {
		sb.WriteString(heading.Render("Shards "))
		sb.WriteString(faint.Render("(blocks verified)"))
		sb.WriteString("\n")
		shards := make([]struct {
			digest string
			count  string
		}, 0, m.shards.Size())
		for shard, count := range m.shards.Iterator() {
			shards = append(shards, struct {
				digest string
				count  string
			}{
				digest: digestutil.Format(shard),
				count:  humanize.Comma(int64(count)),
			})
		}
		slices.SortFunc(shards, func(a, b struct {
			digest string
			count  string
		}) int {
			return strings.Compare(a.digest, b.digest)
		})
		max := 5
		for i := range max {
			if i >= len(shards) {
				break
			}
			shard := shards[i]
			sb.WriteString("  ")
			sb.WriteString(shard.digest)
			sb.WriteString(" (")
			sb.WriteString(shard.count)
			sb.WriteString(")\n")
		}
		if len(shards) > max {
			sb.WriteString("  ...")
			sb.WriteString(humanize.Comma(int64(len(shards) - max)))
			sb.WriteString(" more\n")
		}
	}

	if len(m.origins) > 0 {
		sb.WriteString(heading.Render("Origins"))
		sb.WriteString("\n")
		origins := make([]struct {
			node   string
			host   string
			blocks string
		}, 0, m.shards.Size())
		for node, host := range m.origins {
			origins = append(origins, struct {
				node   string
				host   string
				blocks string
			}{
				node:   node.String(),
				host:   host,
				blocks: humanize.Comma(int64(m.originBlocks[node])),
			})
		}
		slices.SortFunc(origins, func(a, b struct {
			node   string
			host   string
			blocks string
		}) int {
			return strings.Compare(a.node, b.node)
		})
		max := 5
		for i := range max {
			if i >= len(origins) {
				break
			}
			origin := origins[i]
			sb.WriteString("  ")
			sb.WriteString(origin.node)
			sb.WriteString(faint.Render(" @ "))
			sb.WriteString(faint.Render(origin.host))
			sb.WriteString(" (")
			sb.WriteString(origin.blocks)
			sb.WriteString(")\n")
		}
		if len(origins) > max {
			sb.WriteString("  ...")
			sb.WriteString(humanize.Comma(int64(len(origins) - max)))
			sb.WriteString(" more\n")
		}
	}

	sb.WriteString(heading.Render("Blocks "))
	sb.WriteString(faint.Render("verified / known"))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s / %s\n", humanize.Comma(int64(m.vblocks.Size())), humanize.Comma(int64(m.vblocks.Size()+m.blocks.Size()))))

	sb.WriteString(heading.Render("Size"))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n", humanize.IBytes(m.size)))

	sb.WriteString("\n")
	return sb.String()
}

var _ tea.Model = (*verifyModel)(nil)
