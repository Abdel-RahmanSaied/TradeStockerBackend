package natsbus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"

	"tradestocker/internal/telemetry"
)

// startServer runs a NATS server inside the test process, on 127.0.0.1 at
// port (0 picks a free one). Shutdown is the outage; nothing else is touched.
func startServer(t *testing.T, port int) *server.Server {
	t.Helper()
	if port == 0 {
		port = server.RANDOM_PORT
	}
	s, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: port, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("in-process NATS did not start")
	}
	t.Cleanup(s.Shutdown)
	return s
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestURLMustHaveNoCredentials(t *testing.T) {
	password := "fixture-" + fmt.Sprint(time.Now().UnixNano())
	for _, raw := range []string{"nats://user:" + password + "@127.0.0.1:4222", "nats://" + password + "@127.0.0.1:4222", "tls://127.0.0.1:4222",
		"nats://127.0.0.1", "nats://127.0.0.1:4222/x", "nats://127.0.0.1:4222?token=" + password, "127.0.0.1:4222", ""} {
		err := CheckURL(raw)
		if err == nil {
			t.Fatalf("%q accepted", raw)
		}
		if strings.Contains(err.Error(), password) {
			t.Fatalf("error repeats the credential: %v", err)
		}
		if _, err = Connect(raw, "test"); err == nil || strings.Contains(err.Error(), password) {
			t.Fatalf("Connect(%q): %v", raw, err)
		}
	}
	if err := CheckURL("nats://127.0.0.1:4222"); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationCarriesTheTrace(t *testing.T) {
	s := startServer(t, 0)
	sub, err := Connect(s.ClientURL(), "subscriber")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	type got struct {
		trace telemetry.Trace
		ok    bool
		id    string
	}
	received := make(chan got, 2)
	if err = sub.Subscribe(func(ctx context.Context, jobID string) {
		tr, ok := telemetry.FromContext(ctx)
		received <- got{tr, ok, jobID}
	}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "subscriber", sub.Ready)
	if err = sub.conn.Flush(); err != nil {
		t.Fatal(err)
	}
	pub, err := Connect(s.ClientURL(), "publisher")
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	waitFor(t, "publisher", pub.Ready)

	parent := telemetry.NewTrace()
	if err = pub.JobCreated(telemetry.WithTrace(context.Background(), parent), "research_0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	select {
	case g := <-received:
		if !g.ok || g.trace.TraceID != parent.TraceID || g.trace.SpanID == parent.SpanID || g.id != "research_0123456789abcdef0123456789abcdef" {
			t.Fatalf("received %+v, parent %+v", g, parent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification not received")
	}
	// Without a trace header the receiver starts a new trace.
	if err = pub.JobCreated(context.Background(), "research_00000000000000000000000000000001"); err != nil {
		t.Fatal(err)
	}
	select {
	case g := <-received:
		if !g.ok || g.trace.TraceID == parent.TraceID {
			t.Fatalf("untraced message %+v", g)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second notification not received")
	}
}

// An outage shows in Ready and makes publishing fail at once (no buffering);
// the connection comes back on its own when NATS returns.
func TestOutageAndRecovery(t *testing.T) {
	s := startServer(t, 0)
	port := s.Addr().(interface{ String() string }).String()
	_, portText, _ := strings.Cut(port, "127.0.0.1:")
	var portNumber int
	if _, err := fmt.Sscan(portText, &portNumber); err != nil {
		t.Fatal(err)
	}
	b, err := Connect(s.ClientURL(), "outage")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	waitFor(t, "connect", b.Ready)

	s.Shutdown()
	waitFor(t, "disconnect", func() bool { return !b.Ready() })
	start := time.Now()
	if err = b.JobCreated(context.Background(), "research_00000000000000000000000000000002"); err == nil {
		t.Fatal("publish during an outage reported success")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("publish during an outage took %v", d)
	}

	startServer(t, portNumber)
	waitFor(t, "reconnect", b.Ready)
	if err = b.JobCreated(context.Background(), "research_00000000000000000000000000000003"); err != nil {
		t.Fatalf("publish after recovery: %v", err)
	}
}

// Starting before NATS is up does not fail; the bus connects when it can.
func TestConnectBeforeNATSIsUp(t *testing.T) {
	probe := startServer(t, 0)
	url := probe.ClientURL()
	_, portText, _ := strings.Cut(url, "127.0.0.1:")
	var port int
	if _, err := fmt.Sscan(portText, &port); err != nil {
		t.Fatal(err)
	}
	probe.Shutdown()
	b, err := Connect(url, "early")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Ready() {
		t.Fatal("ready with no server")
	}
	startServer(t, port)
	waitFor(t, "late connect", b.Ready)
}
