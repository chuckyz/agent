// Package all imports all known component packages.
package all

import (
	_ "github.com/grafana/agent/component/discovery/kubernetes"                 // Import discovery.kubernetes
	_ "github.com/grafana/agent/component/discovery/relabel"                    // Import discovery.relabel
	_ "github.com/grafana/agent/component/local/file"                           // Import local.file
	_ "github.com/grafana/agent/component/prometheus/integration/node_exporter" // Import prometheus.integration.node_exporter
	_ "github.com/grafana/agent/component/prometheus/relabel"                   // Import prometheus.relabel
	_ "github.com/grafana/agent/component/prometheus/remotewrite"               // Import prometheus.remote_write
	_ "github.com/grafana/agent/component/prometheus/scrape"                    // Import prometheus.scrape
	_ "github.com/grafana/agent/component/remote/s3"                            // Import s3.file
)
