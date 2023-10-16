package scrape

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/agent/component"
	"github.com/grafana/agent/component/prometheus"
	"github.com/grafana/agent/pkg/river"
	"github.com/grafana/agent/pkg/util"
	prometheus_client "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/require"
)

func TestRiverConfig(t *testing.T) {
	var exampleRiverConfig = `
	targets         = [{ "target1" = "target1" }]
	forward_to      = []
	scrape_interval = "10s"
	job_name        = "local"

	bearer_token = "token"
	proxy_url = "http://0.0.0.0:11111"
	follow_redirects = true
	enable_http2 = true

	tls_config {
		ca_file = "/path/to/file.ca"
		cert_file = "/path/to/file.cert"
		key_file = "/path/to/file.key"
		server_name = "server_name"
		insecure_skip_verify = false
		min_version = "TLS13"
	}
`

	var args Arguments
	err := river.Unmarshal([]byte(exampleRiverConfig), &args)
	require.NoError(t, err)
}

func TestBadRiverConfig(t *testing.T) {
	var exampleRiverConfig = `
	targets         = [{ "target1" = "target1" }]
	forward_to      = []
	scrape_interval = "10s"
	job_name        = "local"

	bearer_token = "token"
	bearer_token_file = "/path/to/file.token"
	proxy_url = "http://0.0.0.0:11111"
	follow_redirects = true
	enable_http2 = true
`

	// Make sure the squashed HTTPClientConfig Validate function is being utilized correctly
	var args Arguments
	err := river.Unmarshal([]byte(exampleRiverConfig), &args)
	require.ErrorContains(t, err, "at most one of bearer_token & bearer_token_file must be configured")
}

func TestForwardingToAppendable(t *testing.T) {
	opts := component.Options{
		Logger:     util.TestFlowLogger(t),
		Registerer: prometheus_client.NewRegistry(),
	}

	nilReceivers := []storage.Appendable{nil, nil}

	args := DefaultArguments
	args.ForwardTo = nilReceivers

	s, err := New(opts, args)
	require.NoError(t, err)

	// Forwarding samples to the nil receivers shouldn't fail.
	appender := s.appendable.Appender(context.Background())
	_, err = appender.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)

	err = appender.Commit()
	require.NoError(t, err)

	// Update the component with a mock receiver; it should be passed along to the Appendable.
	var receivedTs int64
	var receivedSamples labels.Labels
	fanout := prometheus.NewInterceptor(nil, prometheus.WithAppendHook(func(ref storage.SeriesRef, l labels.Labels, t int64, _ float64, _ storage.Appender) (storage.SeriesRef, error) {
		receivedTs = t
		receivedSamples = l
		return ref, nil
	}))
	require.NoError(t, err)
	args.ForwardTo = []storage.Appendable{fanout}
	err = s.Update(args)
	require.NoError(t, err)

	// Forwarding a sample to the mock receiver should succeed.
	appender = s.appendable.Appender(context.Background())
	timestamp := time.Now().Unix()
	sample := labels.FromStrings("foo", "bar")
	_, err = appender.Append(0, sample, timestamp, 42.0)
	require.NoError(t, err)

	err = appender.Commit()
	require.NoError(t, err)

	require.Equal(t, receivedTs, timestamp)
	require.Len(t, receivedSamples, 1)
	require.Equal(t, receivedSamples, sample)
}