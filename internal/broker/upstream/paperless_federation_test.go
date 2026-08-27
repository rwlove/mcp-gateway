package upstream

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kuadrant/mcp-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaperlessFederation_Live drives the REAL stock baruchiro/paperless-mcp
// (started separately; its URL in PAPERLESS_MCP_URL) through the broker's
// upstream client to prove the end-to-end behaviour this change enables.
//
// Stock paperless-mcp runs the streamable-HTTP transport in stateless mode and,
// on an older MCP SDK, down-negotiates to protocol 2025-03-26 with no
// Mcp-Session-Id. Before this change the broker treats it as stateful, pings its
// (non-existent) session, and de-federates it. With session-less handling it
// connects, stays ready, and its tools are fetched — and, advertising a 2025
// revision, it is routed onto the stateful served set.
//
// Skipped unless PAPERLESS_MCP_URL is set, e.g.:
//
//	PAPERLESS_URL=http://example.test PAPERLESS_API_KEY=x \
//	  node build/index.js --http --port 3210 --no-auth &
//	PAPERLESS_MCP_URL=http://127.0.0.1:3210/mcp \
//	  go test -run TestPaperlessFederation_Live ./internal/broker/upstream/
func TestPaperlessFederation_Live(t *testing.T) {
	url := os.Getenv("PAPERLESS_MCP_URL")
	if url == "" {
		t.Skip("set PAPERLESS_MCP_URL to a running stock paperless-mcp (e.g. http://127.0.0.1:3210/mcp)")
	}

	up := NewUpstreamMCP(&config.MCPServer{
		Name:   "paperless",
		Prefix: "pl_",
		URL:    url,
	}, "", slog.Default())

	gw := newMockToolsAdderDeleter()
	mgr, err := NewUpstreamMCPManager(up, gw, nil, slog.Default(), 0, InvalidToolPolicyFilterOut)
	require.NoError(t, err)

	active := mgr.Start(context.Background())
	defer active.Stop()

	require.Eventually(t, func() bool { return mgr.GetStatus().Ready },
		25*time.Second, 250*time.Millisecond,
		"stock paperless upstream should connect and become ready")

	versions := up.SupportedVersions()
	assert.Contains(t, versions, "2025-03-26",
		"stock paperless negotiates 2025-03-26")
	assert.True(t, up.IsSessionless(),
		"stock paperless issues no Mcp-Session-Id (stateless transport)")
	assert.NotEmpty(t, gw.tools,
		"paperless tools should be fetched and registered on the gateway")

	hasStatefulFamily := false
	for _, v := range versions {
		if strings.HasPrefix(v, "2025-") {
			hasStatefulFamily = true
		}
	}
	assert.True(t, hasStatefulFamily,
		"advertises a 2025-family version → served on the stateful route")

	t.Logf("federated stock paperless: versions=%v tools=%d sessionless=%v",
		versions, len(gw.tools), up.IsSessionless())
}
