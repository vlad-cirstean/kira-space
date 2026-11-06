package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/clientcmd"
)

// classify maps a client error to a Status reason; "" means no specific cluster trouble.
func classify(err error) string {
	switch {
	case err == nil:
		return ""
	case apierrors.IsUnauthorized(err):
		return ReasonUnauthorized
	case apierrors.IsForbidden(err):
		return ReasonForbidden
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		return ""
	}
	msg := err.Error()
	switch {
	case isPluginMessage(msg):
		return ReasonPlugin
	case clientcmd.IsConfigurationInvalid(err), clientcmd.IsEmptyConfig(err), clientcmd.IsContextNotFound(err),
		strings.Contains(msg, "auth provider"), strings.Contains(msg, "auth-provider"):
		return ReasonConfig
	}
	var nerr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()) {
		return ReasonTimeout
	}
	var uerr *url.Error
	var operr *net.OpError
	if errors.As(err, &uerr) || errors.As(err, &operr) || errors.Is(err, net.ErrClosed) {
		return ReasonUnreachable
	}
	return ""
}

func isPluginMessage(msg string) bool {
	for _, s := range []string{"getting credentials", "exec plugin", "credential plugin", "exec:", "standard input is"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func isInteractiveMessage(msg string) bool { return strings.Contains(msg, "standard input is") }

// mapErr turns a client error into the structured error bound methods return.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie
	}
	msg := err.Error()
	params := map[string]string{"message": msg}
	reasoned := func(code ipcerr.Code, key, reason string) error {
		e := ipcerr.T(code, key, params)
		e.Details, _ = json.Marshal(map[string]string{"reason": reason})
		return e
	}
	switch classify(err) {
	case ReasonUnauthorized:
		return ipcerr.T(ipcerr.CodeUnauthorized, "backend.studio.kubernetes.unauthorized.error", params)
	case ReasonForbidden:
		return ipcerr.T(ipcerr.CodeForbidden, "backend.studio.kubernetes.forbidden.error", params)
	case ReasonPlugin:
		if isInteractiveMessage(msg) {
			return reasoned(ipcerr.CodeUnavailable, "backend.studio.kubernetes.pluginInteractive.error", ReasonPlugin)
		}
		return reasoned(ipcerr.CodeUnavailable, "backend.studio.kubernetes.plugin.error", ReasonPlugin)
	case ReasonConfig:
		return reasoned(ipcerr.CodeUnavailable, "backend.studio.kubernetes.config.error", ReasonConfig)
	case ReasonTimeout:
		return ipcerr.T(ipcerr.CodeTimeout, "backend.studio.kubernetes.timeout.error", params)
	case ReasonUnreachable:
		return reasoned(ipcerr.CodeUnavailable, "backend.studio.kubernetes.unreachable.error", ReasonUnreachable)
	}
	if apierrors.IsNotFound(err) {
		return ipcerr.T(ipcerr.CodeNotFound, "backend.studio.kubernetes.notFound.error", params)
	}
	if apierrors.IsBadRequest(err) {
		return ipcerr.BadRequest(msg)
	}
	return ipcerr.Internal(msg)
}

func unknownCluster() error {
	return ipcerr.T(ipcerr.CodeNotFound, "backend.studio.kubernetes.notFound.error", map[string]string{"message": "unknown cluster"})
}
