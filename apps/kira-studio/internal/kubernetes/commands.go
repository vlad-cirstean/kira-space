package kubernetes

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// The command text is shown in approval dialogs and recorded in the op log; kubectl never runs it.

var plainWord = regexp.MustCompile(`^[A-Za-z0-9._/:=,-]+$`)

func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return loginshell.Quote(s)
}

// kubectlResource turns a kind key (v1/pods, apps/v1/deployments) into kubectl's resource.group.
func kubectlResource(key string) string {
	parts := strings.Split(key, "/")
	switch len(parts) {
	case 2:
		return parts[1]
	case 3:
		return parts[2] + "." + parts[0]
	}
	return key
}

func withContext(b []string, ctxName string) string {
	return strings.Join(append(b, "--context", shellWord(ctxName)), " ")
}

// StatusCommand is the probe's command line.
func StatusCommand(ctxName string) string { return withContext([]string{"kubectl version"}, ctxName) }

// NamespacesCommand is Namespaces' command line.
func NamespacesCommand(ctxName string) string {
	return withContext([]string{"kubectl get namespaces"}, ctxName)
}

// KindsCommand is Kinds' command line.
func KindsCommand(ctxName string) string {
	return withContext([]string{"kubectl api-resources"}, ctxName)
}

// ListCommand is List's command line.
func ListCommand(ctxName string, a ListArgs) string {
	b := []string{"kubectl get", shellWord(kubectlResource(a.Kind))}
	if a.Namespace == "" {
		b = append(b, "-A")
	} else {
		b = append(b, "-n", shellWord(a.Namespace))
	}
	if a.LabelSelector != "" {
		b = append(b, "-l", shellWord(a.LabelSelector))
	}
	return withContext(b, ctxName)
}

// DescribeCommand is Describe's command line.
func DescribeCommand(ctxName string, a DescribeArgs) string {
	b := []string{"kubectl describe", shellWord(kubectlResource(a.Kind)), shellWord(a.Name)}
	if a.Namespace != "" {
		b = append(b, "-n", shellWord(a.Namespace))
	}
	return withContext(b, ctxName)
}

// LogsCommand is the logs command line; tail -1 means all lines.
func LogsCommand(ctxName string, a LogsReadArgs, follow bool) string {
	b := []string{"kubectl logs", shellWord(a.Pod), "-n", shellWord(a.Namespace)}
	if a.Container != "" {
		b = append(b, "-c", shellWord(a.Container))
	}
	if a.Previous {
		b = append(b, "--previous")
	}
	b = append(b, "--tail", strconv.Itoa(a.Tail))
	if a.Timestamps {
		b = append(b, "--timestamps")
	}
	if follow {
		b = append(b, "-f")
	}
	return withContext(b, ctxName)
}
