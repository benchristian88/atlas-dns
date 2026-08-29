package haoperations

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"time"
)

type dnsProbeOutcome struct {
	result DNSProbeResult
	err    error
}

type sequenceDNSProber struct {
	outcomes []dnsProbeOutcome
	calls    int
}

func (p *sequenceDNSProber) Probe(context.Context, DNSProbeRequest) (DNSProbeResult, error) {
	if p.calls >= len(p.outcomes) {
		return DNSProbeResult{}, errors.New("unexpected DNS probe call")
	}
	outcome := p.outcomes[p.calls]
	p.calls++
	return outcome.result, outcome.err
}

func failedDNSProbe(code string) dnsProbeOutcome {
	return dnsProbeOutcome{
		result: DNSProbeResult{Status: "failed", UDPStatus: "failed", TCPStatus: "failed", ErrorCode: code},
		err:    errors.New("DNS service probe failed"),
	}
}

func healthyDNSProbe() dnsProbeOutcome {
	return dnsProbeOutcome{result: DNSProbeResult{Status: "healthy", UDPStatus: "healthy", TCPStatus: "healthy"}}
}

func TestConfirmingDNSProberRecoversTransientFailureWithoutFullScheduleDelay(t *testing.T) {
	base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{failedDNSProbe("DNS_PROBE_TIMEOUT"), healthyDNSProbe()}}
	waits := []time.Duration{}
	clock := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	prober := newConfirmingDNSProber(base, defaultDNSProbeConfirmationBackoffs, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		clock = clock.Add(delay)
		return nil
	}, func() time.Time { return clock })

	result, err := prober.Probe(context.Background(), DNSProbeRequest{UDP: true, TCP: true})
	if err != nil || result.Status != "healthy" || result.dnsProbeAttempts != 2 || !result.dnsProbeRecoveredAfterRetry {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, base.calls)
	}
	if !reflect.DeepEqual(waits, []time.Duration{250 * time.Millisecond}) || result.dnsProbeElapsed != 250*time.Millisecond {
		t.Fatalf("waits=%v elapsed=%s", waits, result.dnsProbeElapsed)
	}
}

func TestConfirmingDNSProberConfirmsPersistentFailureThreeTimes(t *testing.T) {
	base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{
		failedDNSProbe("DNS_PROBE_UNREACHABLE"),
		failedDNSProbe("DNS_PROBE_UNREACHABLE"),
		failedDNSProbe("DNS_PROBE_UNREACHABLE"),
	}}
	waits := []time.Duration{}
	clock := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	prober := newConfirmingDNSProber(base, defaultDNSProbeConfirmationBackoffs, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		clock = clock.Add(delay)
		return nil
	}, func() time.Time { return clock })

	result, err := prober.Probe(context.Background(), DNSProbeRequest{UDP: true, TCP: true})
	if err == nil || result.Status != "failed" || result.dnsProbeAttempts != 3 || result.dnsProbeRecoveredAfterRetry {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, base.calls)
	}
	if !reflect.DeepEqual(waits, defaultDNSProbeConfirmationBackoffs) || result.dnsProbeElapsed != 750*time.Millisecond {
		t.Fatalf("waits=%v elapsed=%s", waits, result.dnsProbeElapsed)
	}
}

func TestConfirmingDNSProberHealthyPathHasNoRetryTraffic(t *testing.T) {
	base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{healthyDNSProbe()}}
	waits := 0
	prober := newConfirmingDNSProber(base, defaultDNSProbeConfirmationBackoffs, func(context.Context, time.Duration) error {
		waits++
		return nil
	}, time.Now)

	result, err := prober.Probe(context.Background(), DNSProbeRequest{UDP: true, TCP: true})
	if err != nil || result.Status != "healthy" || result.dnsProbeAttempts != 1 || base.calls != 1 || waits != 0 {
		t.Fatalf("result=%#v err=%v calls=%d waits=%d", result, err, base.calls, waits)
	}
}

func TestConfirmingDNSProberRetryClassification(t *testing.T) {
	for _, code := range []string{"DNS_PROBE_TIMEOUT", "DNS_PROBE_UNREACHABLE", "DNS_PROBE_UNEXPECTED_RCODE", "DNS_PROBE_FAILED"} {
		t.Run(code, func(t *testing.T) {
			base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{failedDNSProbe(code), healthyDNSProbe()}}
			prober := newConfirmingDNSProber(base, []time.Duration{0}, waitForDNSProbeConfirmation, time.Now)
			if result, err := prober.Probe(context.Background(), DNSProbeRequest{}); err != nil || result.Status != "healthy" || base.calls != 2 {
				t.Fatalf("result=%#v err=%v calls=%d", result, err, base.calls)
			}
		})
	}

	t.Run("unclassified local failure", func(t *testing.T) {
		base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{{result: DNSProbeResult{Status: "failed"}, err: errors.New("DNS probe name is invalid")}}}
		prober := newConfirmingDNSProber(base, defaultDNSProbeConfirmationBackoffs, waitForDNSProbeConfirmation, time.Now)
		if _, err := prober.Probe(context.Background(), DNSProbeRequest{}); err == nil || base.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, base.calls)
		}
	})
}

func TestConfirmingDNSProberCancellationInterruptsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	base := &sequenceDNSProber{outcomes: []dnsProbeOutcome{failedDNSProbe("DNS_PROBE_TIMEOUT")}}
	waits := 0
	prober := newConfirmingDNSProber(base, defaultDNSProbeConfirmationBackoffs, func(ctx context.Context, delay time.Duration) error {
		waits++
		cancel()
		return waitForDNSProbeConfirmation(ctx, delay)
	}, time.Now)

	started := time.Now()
	result, err := prober.Probe(ctx, DNSProbeRequest{})
	if !errors.Is(err, context.Canceled) || base.calls != 1 || waits != 1 || result.dnsProbeAttempts != 1 {
		t.Fatalf("result=%#v err=%v calls=%d waits=%d", result, err, base.calls, waits)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

func TestDNSQueryBuildsBoundedRootNSRequest(t *testing.T) {
	message, id, err := dnsQuery(".", "NS")
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 17 {
		t.Fatalf("length=%d", len(message))
	}
	if binary.BigEndian.Uint16(message[:2]) != id || binary.BigEndian.Uint16(message[4:6]) != 1 {
		t.Fatalf("invalid header %x", message[:12])
	}
	if binary.BigEndian.Uint16(message[13:15]) != 2 {
		t.Fatalf("type=%d", binary.BigEndian.Uint16(message[13:15]))
	}
}

func TestValidateDNSResponseRequiresMatchingResponse(t *testing.T) {
	response := make([]byte, 12)
	binary.BigEndian.PutUint16(response[:2], 42)
	binary.BigEndian.PutUint16(response[2:4], 0x8003)
	rcode, _, _, err := validateDNSResponse(response, 42, 5, "ipv4")
	if err != nil || rcode != 3 {
		t.Fatalf("rcode=%d err=%v", rcode, err)
	}
	if _, _, _, err := validateDNSResponse(response, 43, 5, "ipv4"); err == nil {
		t.Fatal("mismatched ID accepted")
	}
}

func TestWireDNSProberRequiresEveryEnabledProtocol(t *testing.T) {
	t.Run("UDP success and TCP failure", func(t *testing.T) {
		prober := NewWireDNSProber(100 * time.Millisecond)
		prober.exchangeFn = func(_ context.Context, network, _ string, _ []byte, _ uint16) (int, int, string, error) {
			if network == "udp" {
				return 0, 2, "ipv4", nil
			}
			return -1, 0, "ipv4", errors.New("connection refused")
		}
		result, probeErr := prober.Probe(context.Background(), DNSProbeRequest{Host: "192.0.2.10", Port: 53, Name: ".", Type: "NS", UDP: true, TCP: true})
		if probeErr == nil || result.Status != "failed" || result.UDPStatus != "healthy" || result.TCPStatus != "failed" {
			t.Fatalf("result=%#v err=%v", result, probeErr)
		}
	})

	t.Run("UDP failure and TCP success", func(t *testing.T) {
		prober := NewWireDNSProber(100 * time.Millisecond)
		prober.exchangeFn = func(_ context.Context, network, _ string, _ []byte, _ uint16) (int, int, string, error) {
			if network == "udp" {
				return -1, 0, "ipv4", timeoutProbeError{}
			}
			return 0, 3, "ipv4", nil
		}
		result, probeErr := prober.Probe(context.Background(), DNSProbeRequest{Host: "192.0.2.10", Port: 53, Name: ".", Type: "NS", UDP: true, TCP: true})
		if probeErr == nil || result.Status != "failed" || result.UDPStatus != "failed" || result.TCPStatus != "healthy" || result.ErrorCode != "DNS_PROBE_TIMEOUT" {
			t.Fatalf("result=%#v err=%v", result, probeErr)
		}
	})
}

func TestWireDNSProberRunsRequiredProtocolsInOneBoundedAttemptWindow(t *testing.T) {
	prober := NewWireDNSProber(2 * time.Second)
	started := make(chan string, 2)
	release := make(chan struct{})
	prober.exchangeFn = func(_ context.Context, network, _ string, _ []byte, _ uint16) (int, int, string, error) {
		started <- network
		<-release
		return 0, 1, "ipv4", nil
	}
	done := make(chan error, 1)
	go func() {
		result, err := prober.Probe(context.Background(), DNSProbeRequest{Host: "192.0.2.10", Port: 53, Name: ".", Type: "NS", UDP: true, TCP: true})
		if err == nil && result.Status != "healthy" {
			err = errors.New("concurrent logical probe was not healthy")
		}
		done <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case protocol := <-started:
			seen[protocol] = true
		case <-time.After(100 * time.Millisecond):
			close(release)
			t.Fatalf("protocols did not start concurrently: %v", seen)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type timeoutProbeError struct{}

func (timeoutProbeError) Error() string   { return "timeout" }
func (timeoutProbeError) Timeout() bool   { return true }
func (timeoutProbeError) Temporary() bool { return true }

func TestDNSErrorCodeClassifiesPersistedEvidence(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		actual   int
		expected int
		want     string
	}{
		{name: "timeout", err: timeoutProbeError{}, want: "DNS_PROBE_TIMEOUT"},
		{name: "unreachable or malformed", err: errors.New("connection refused"), want: "DNS_PROBE_UNREACHABLE"},
		{name: "unexpected response code", actual: 2, expected: 0, want: "DNS_PROBE_UNEXPECTED_RCODE"},
		{name: "fallback", actual: 0, expected: 0, want: "DNS_PROBE_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dnsErrorCode(test.err, test.actual, test.expected); got != test.want {
				t.Fatalf("dnsErrorCode()=%q want %q", got, test.want)
			}
		})
	}
}

func TestVersionAndInstallationClassification(t *testing.T) {
	if SupportForInstallation(InstallationNativeSystemd) != UpgradeGuided || SupportForInstallation(InstallationDocker) != UpgradeGuided {
		t.Fatal("guided installations misclassified")
	}
	if SupportForInstallation(InstallationHomeAssistant) != UpgradeUnsupported || SupportForInstallation(InstallationUnknown) != UpgradeUnsupported {
		t.Fatal("unsafe installation advertised")
	}
	if compareVersions("v0.107.78", "v0.107.79") >= 0 || compareVersions("v0.107.78", "v0.107.78") != 0 {
		t.Fatal("version comparison failed")
	}
}
