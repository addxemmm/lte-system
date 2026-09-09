package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

func TestDirectAPISocketDisabledByDefault(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	c := config.Default()
	c.UIListenAddr = "127.0.0.1:0"
	c.ListenAddr = occupied.Addr().String()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	s, err := bindServers(c, h, h)
	if err != nil || len(s) != 1 {
		t.Fatalf("disabled API tried to bind: %v", err)
	}
	defer s[0].listener.Close()
}

func TestDualListenerFailureReleasesFirstSocket(t *testing.T) {
	block, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer block.Close()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	probe.Close()
	c := config.Default()
	c.UIListenAddr = address
	c.ListenAddr = block.Addr().String()
	c.ExposeAPI = true
	if _, err := bindServers(c, http.NotFoundHandler(), http.NotFoundHandler()); err == nil {
		t.Fatal("occupied API port accepted")
	}
	rebound, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("failed start leaked UI socket")
	}
	rebound.Close()
}

func TestDualListenersServeAndDrainTogether(t *testing.T) {
	c := config.Default()
	c.UIListenAddr = "127.0.0.1:0"
	c.ListenAddr = "127.0.0.1:0"
	c.ExposeAPI = true
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	servers, err := bindServers(c, h, h)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveUntilStopped(ctx, servers) }()
	client := http.Client{Timeout: time.Second}
	for _, s := range servers {
		response, err := client.Get("http://" + s.listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 204 {
			t.Fatal("wrong listener handler")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listeners did not shut down")
	}
}

func TestUnexpectedListenerFailureDrainsPeer(t *testing.T) {
	c := config.Default()
	c.UIListenAddr = "127.0.0.1:0"
	c.ListenAddr = "127.0.0.1:0"
	c.ExposeAPI = true
	servers, err := bindServers(c, http.NotFoundHandler(), http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	servers[1].listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := serveUntilStopped(ctx, servers); err == nil {
		t.Fatal("lost permanent listener failure")
	}
	conn, err := net.DialTimeout("tcp", servers[0].listener.Addr().String(), time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("peer listener leaked")
	}
}
