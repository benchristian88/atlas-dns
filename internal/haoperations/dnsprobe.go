package haoperations

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const maxDNSMessage = 4096

var defaultDNSProbeConfirmationBackoffs = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond}

type DNSProbeRequest struct {
	Host          string
	Port          int
	Name          string
	Type          string
	ExpectedRCode int
	UDP           bool
	TCP           bool
}

type DNSProber interface {
	Probe(context.Context, DNSProbeRequest) (DNSProbeResult, error)
}

type confirmingDNSProber struct {
	probe    DNSProber
	backoffs []time.Duration
	wait     func(context.Context, time.Duration) error
	now      func() time.Time
}

// NewConfirmingDNSProber wraps one complete configured DNS probe in bounded,
// same-cycle confirmation. A healthy first attempt returns immediately; a
// classified failure is retried twice before it can reach state transitions.
func NewConfirmingDNSProber(probe DNSProber) DNSProber {
	return newConfirmingDNSProber(probe, defaultDNSProbeConfirmationBackoffs, waitForDNSProbeConfirmation, time.Now)
}

func newConfirmingDNSProber(probe DNSProber, backoffs []time.Duration, wait func(context.Context, time.Duration) error, now func() time.Time) *confirmingDNSProber {
	return &confirmingDNSProber{probe: probe, backoffs: append([]time.Duration(nil), backoffs...), wait: wait, now: now}
}

func (p *confirmingDNSProber) Probe(ctx context.Context, request DNSProbeRequest) (DNSProbeResult, error) {
	started := p.now()
	var result DNSProbeResult
	var probeErr error
	var priorErrorCode string
	var priorProtocols []string
	for attempt := 1; attempt <= len(p.backoffs)+1; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if p.probe == nil {
			return result, errors.New("DNS prober is not configured")
		}
		result, probeErr = p.probe.Probe(ctx, request)
		result.dnsProbeAttempts = attempt
		result.dnsProbeElapsed = nonNegativeDuration(p.now().Sub(started))
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if probeErr == nil && result.Status == "healthy" {
			result.dnsProbeRecoveredAfterRetry = attempt > 1
			result.dnsProbePriorErrorCode = priorErrorCode
			result.dnsProbePriorProtocols = append([]string(nil), priorProtocols...)
			return result, nil
		}
		priorErrorCode = result.ErrorCode
		priorProtocols = failedDNSProbeProtocols(result)
		if attempt > len(p.backoffs) || !retryableDNSProbeFailure(ctx, result, probeErr) {
			return result, probeErr
		}
		if err := p.wait(ctx, p.backoffs[attempt-1]); err != nil {
			result.dnsProbeElapsed = nonNegativeDuration(p.now().Sub(started))
			return result, err
		}
	}
	return result, probeErr
}

func retryableDNSProbeFailure(ctx context.Context, result DNSProbeResult, probeErr error) bool {
	if ctx.Err() != nil || errors.Is(probeErr, context.Canceled) || errors.Is(probeErr, context.DeadlineExceeded) {
		return false
	}
	switch result.ErrorCode {
	case "DNS_PROBE_TIMEOUT", "DNS_PROBE_UNREACHABLE", "DNS_PROBE_UNEXPECTED_RCODE", "DNS_PROBE_FAILED":
		return true
	default:
		return false
	}
}

func waitForDNSProbeConfirmation(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}

type WireDNSProber struct {
	timeout    time.Duration
	now        func() time.Time
	exchangeFn func(context.Context, string, string, []byte, uint16) (int, int, string, error)
}

func NewWireDNSProber(timeout time.Duration) *WireDNSProber {
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 2 * time.Second
	}
	return &WireDNSProber{timeout: timeout, now: time.Now}
}

func (p *WireDNSProber) Probe(ctx context.Context, request DNSProbeRequest) (DNSProbeResult, error) {
	result := DNSProbeResult{Status: "failed", UDPStatus: "disabled", TCPStatus: "disabled", ProbedAt: p.now().UTC()}
	message, id, err := dnsQuery(request.Name, request.Type)
	if err != nil {
		return result, err
	}
	address := net.JoinHostPort(request.Host, fmt.Sprint(request.Port))
	type protocolOutcome struct {
		rcode   int
		latency int
		family  string
		err     error
	}
	type namedProtocolOutcome struct {
		protocol string
		outcome  protocolOutcome
	}
	protocols := []string{}
	if request.UDP {
		protocols = append(protocols, "udp")
	}
	if request.TCP {
		protocols = append(protocols, "tcp")
	}
	outcomeChannel := make(chan namedProtocolOutcome, len(protocols))
	for _, protocol := range protocols {
		protocol := protocol
		go func() {
			rcode, latency, family, probeErr := p.runExchange(ctx, protocol, address, message, id)
			outcomeChannel <- namedProtocolOutcome{protocol: protocol, outcome: protocolOutcome{rcode: rcode, latency: latency, family: family, err: probeErr}}
		}()
	}
	outcomes := map[string]protocolOutcome{}
	for range protocols {
		value := <-outcomeChannel
		outcomes[value.protocol] = value.outcome
	}

	var successes, latencyTotal int
	if request.UDP {
		outcome := outcomes["udp"]
		if outcome.err == nil && outcome.rcode == request.ExpectedRCode {
			result.UDPStatus, result.ResponseCode, result.AddressFamily = "healthy", intPointer(outcome.rcode), outcome.family
			successes++
			latencyTotal += outcome.latency
		} else {
			result.UDPStatus = "failed"
			result.ErrorCode = dnsErrorCode(outcome.err, outcome.rcode, request.ExpectedRCode)
		}
	}
	if request.TCP {
		outcome := outcomes["tcp"]
		if outcome.err == nil && outcome.rcode == request.ExpectedRCode {
			result.TCPStatus, result.ResponseCode = "healthy", intPointer(outcome.rcode)
			if result.AddressFamily == "" {
				result.AddressFamily = outcome.family
			}
			successes++
			latencyTotal += outcome.latency
		} else {
			result.TCPStatus = "failed"
			if result.ErrorCode == "" {
				result.ErrorCode = dnsErrorCode(outcome.err, outcome.rcode, request.ExpectedRCode)
			}
		}
	}
	required := len(protocols)
	if required > 0 && successes == required {
		result.Status, result.ErrorCode = "healthy", ""
		latency := latencyTotal / successes
		result.LatencyMS = &latency
		return result, nil
	}
	return result, errors.New("DNS service probe failed")
}

func (p *WireDNSProber) runExchange(ctx context.Context, network, address string, message []byte, id uint16) (int, int, string, error) {
	if p.exchangeFn != nil {
		return p.exchangeFn(ctx, network, address, message, id)
	}
	return p.exchange(ctx, network, address, message, id)
}

func (p *WireDNSProber) exchange(ctx context.Context, network, address string, message []byte, id uint16) (int, int, string, error) {
	dialer := net.Dialer{Timeout: p.timeout}
	started := p.now()
	connection, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return -1, 0, "", err
	}
	defer connection.Close()
	deadline := p.now().Add(p.timeout)
	_ = connection.SetDeadline(deadline)
	family := "ipv4"
	if host, _, splitErr := net.SplitHostPort(connection.RemoteAddr().String()); splitErr == nil && strings.Contains(host, ":") {
		family = "ipv6"
	}
	if network == "tcp" {
		framed := make([]byte, len(message)+2)
		binary.BigEndian.PutUint16(framed[:2], uint16(len(message)))
		copy(framed[2:], message)
		if _, err = connection.Write(framed); err != nil {
			return -1, 0, family, err
		}
		var size [2]byte
		if _, err = io.ReadFull(connection, size[:]); err != nil {
			return -1, 0, family, err
		}
		length := int(binary.BigEndian.Uint16(size[:]))
		if length < 12 || length > maxDNSMessage {
			return -1, 0, family, errors.New("invalid DNS response size")
		}
		response := make([]byte, length)
		if _, err = io.ReadFull(connection, response); err != nil {
			return -1, 0, family, err
		}
		return validateDNSResponse(response, id, int(p.now().Sub(started).Milliseconds()), family)
	}
	if _, err = connection.Write(message); err != nil {
		return -1, 0, family, err
	}
	response := make([]byte, maxDNSMessage)
	read, err := connection.Read(response)
	if err != nil {
		return -1, 0, family, err
	}
	return validateDNSResponse(response[:read], id, int(p.now().Sub(started).Milliseconds()), family)
}

func dnsQuery(name, recordType string) ([]byte, uint16, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, 0, errors.New("DNS probe name is empty")
	}
	var idBytes [2]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return nil, 0, err
	}
	id := binary.BigEndian.Uint16(idBytes[:])
	message := make([]byte, 12, 512)
	binary.BigEndian.PutUint16(message[0:2], id)
	binary.BigEndian.PutUint16(message[2:4], 0x0100)
	binary.BigEndian.PutUint16(message[4:6], 1)
	if name == "." {
		message = append(message, 0)
	} else {
		for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
			if label == "" || len(label) > 63 {
				return nil, 0, errors.New("DNS probe name is invalid")
			}
			message = append(message, byte(len(label)))
			message = append(message, label...)
		}
		message = append(message, 0)
	}
	typeCode := uint16(2)
	switch recordType {
	case "A":
		typeCode = 1
	case "AAAA":
		typeCode = 28
	case "NS":
	default:
		return nil, 0, errors.New("DNS probe type is invalid")
	}
	message = binary.BigEndian.AppendUint16(message, typeCode)
	message = binary.BigEndian.AppendUint16(message, 1)
	return message, id, nil
}

func validateDNSResponse(response []byte, id uint16, latency int, family string) (int, int, string, error) {
	if len(response) < 12 {
		return -1, latency, family, errors.New("DNS response is truncated")
	}
	if binary.BigEndian.Uint16(response[:2]) != id {
		return -1, latency, family, errors.New("DNS response ID does not match")
	}
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x8000 == 0 {
		return -1, latency, family, errors.New("DNS response flag is absent")
	}
	return int(flags & 0x000f), latency, family, nil
}

func dnsErrorCode(err error, actual, expected int) string {
	if err != nil {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			return "DNS_PROBE_TIMEOUT"
		}
		return "DNS_PROBE_UNREACHABLE"
	}
	if actual != expected {
		return "DNS_PROBE_UNEXPECTED_RCODE"
	}
	return "DNS_PROBE_FAILED"
}

func intPointer(value int) *int { return &value }
