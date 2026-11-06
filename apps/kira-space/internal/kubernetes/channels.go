package kubernetes

// ChannelLogs carries LogsEvent batches. Only this module emits it.
const ChannelLogs = "kira:kubernetes:logs"

// ChannelClusters tells windows a cluster setting changed (order, MCP); listeners refetch Clusters.
const ChannelClusters = "kira:kubernetes:clusters"
