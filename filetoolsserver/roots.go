//lint:file-ignore SA1019 R20 intentionally preserves deprecated MCP roots during the compatibility window and removes logging from modern discovery.
package filetoolsserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/security"
)

const methodDiscover = "server/discover"

func createDiscoveryMiddleware(h *handler.Handler, enableClientRoots bool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodDiscover {
				return next(ctx, method, req)
			}
			if enableClientRoots && !h.HasConfiguredDirectories() {
				return nil, &jsonrpc.Error{
					Code:    jsonrpc.CodeMethodNotFound,
					Message: "server discovery is unavailable while legacy client roots are required",
				}
			}

			result, err := next(ctx, method, req)
			if err != nil {
				return nil, err
			}
			discovery, ok := result.(*mcp.DiscoverResult)
			if !ok || discovery.Capabilities == nil {
				return result, nil
			}

			capabilities := *discovery.Capabilities
			capabilities.Logging = nil
			discovery.Capabilities = &capabilities
			return discovery, nil
		}
	}
}

func createInitializedHandler(lifecycleCtx context.Context, h *handler.Handler, version string, enableClientRoots bool) func(context.Context, *mcp.InitializedRequest) {
	return func(ctx context.Context, req *mcp.InitializedRequest) {
		// The update check belongs to the process lifecycle, not an independent
		// background context that can outlive server shutdown.
		go handler.CheckForUpdatesAsync(lifecycleCtx, req.Session, version)

		if !enableClientRoots {
			return
		}
		if h.HasConfiguredDirectories() {
			slog.Debug("skipping MCP roots request because process directories are configured",
				"dirs", h.GetAllowedDirectories())
			return
		}
		if !clientSupportsRoots(req.Session) {
			warnNoAllowedDirectories()
			return
		}

		result, err := req.Session.ListRoots(ctx, &mcp.ListRootsParams{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to request roots from client: %v\n", err)
			return
		}
		updateAllowedDirectoriesFromRoots(h, result.Roots)
		if len(h.GetAllowedDirectories()) == 0 {
			warnNoAllowedDirectories()
		}
	}
}

func createRootsListChangedHandler(h *handler.Handler, enableClientRoots bool) func(context.Context, *mcp.RootsListChangedRequest) {
	return func(ctx context.Context, req *mcp.RootsListChangedRequest) {
		if !enableClientRoots || h.HasConfiguredDirectories() || !clientSupportsRoots(req.Session) {
			return
		}

		result, err := req.Session.ListRoots(ctx, &mcp.ListRootsParams{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to request updated roots from client: %v\n", err)
			return
		}
		updateAllowedDirectoriesFromRoots(h, result.Roots)
	}
}

func clientSupportsRoots(session *mcp.ServerSession) bool {
	params := session.InitializeParams()
	return params != nil && params.Capabilities != nil && params.Capabilities.RootsV2 != nil
}

func warnNoAllowedDirectories() {
	fmt.Fprintln(os.Stderr, "Warning: No allowed directories configured. File operations will fail.")
	fmt.Fprintln(os.Stderr, "Provide directories via CLI arguments or ensure MCP client supports roots protocol.")
}

// fileURIToPath converts a file:// URI to a local filesystem path.
func fileURIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}

	path := parsed.Path
	// Windows: url.Parse turns file:///C:/path into /C:/path — strip the leading slash
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}

	return path
}

func updateAllowedDirectoriesFromRoots(h *handler.Handler, roots []*mcp.Root) {
	validatedDirs := make([]string, 0, len(roots))

	for _, root := range roots {
		rootPath := fileURIToPath(root.URI)

		normalized, err := security.NormalizeAllowedDirs([]string{rootPath})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to normalize root directory %s: %v\n", rootPath, err)
			continue
		}

		if len(normalized) > 0 {
			// Keep the client-provided absolute spelling so lexical containment
			// remains valid for platform aliases such as /var -> /private/var.
			// Handler normalization separately retains the resolved destination.
			validatedDirs = append(validatedDirs, rootPath)
		}
	}

	merged := h.MergeAllowedDirectories(validatedDirs)
	if len(validatedDirs) > 0 {
		slog.Debug("merged allowed directories from MCP roots",
			"roots", validatedDirs, "merged", merged)
		return
	}
	slog.Debug("cleared dynamic MCP roots", "configured", merged)
}
