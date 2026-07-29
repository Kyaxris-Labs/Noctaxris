package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	mqttBridgeStartHook   func(*Server) error
	mqttBridgeRunningHook func()
)

// SetMQTTBridgeStartHookForTest replaces live MQTT bridge startup (unit tests).
func SetMQTTBridgeStartHookForTest(hook func(*Server) error) {
	mqttBridgeStartHook = hook
}

// SetMQTTBridgeRunningHookForTest records when the live bridge connects.
func SetMQTTBridgeRunningHookForTest(hook func()) {
	mqttBridgeRunningHook = hook
}

func (s *Server) startSharedMQTTIfEnabled() {
	if s == nil || !s.cfg.SharedMQTT {
		return
	}
	if mqttBridgeStartHook != nil {
		if err := mqttBridgeStartHook(s); err != nil {
			log.Printf("shared mqtt: %v", err)
		}
		return
	}
	go func() {
		if err := s.runSharedMQTTBridge(context.Background()); err != nil && !errorsIsContextCanceled(err) {
			log.Printf("shared mqtt bridge: %v", err)
		}
	}()
}

func errorsIsContextCanceled(err error) bool {
	return err != nil && (err == context.Canceled || strings.Contains(err.Error(), context.Canceled.Error()))
}

func (s *Server) runSharedMQTTBridge(ctx context.Context) error {
	if !compute.BrokerPortPublishReady() {
		return fmt.Errorf("shared mqtt requires %s or %s for API dial", compute.EnvBrokerPortPublish, compute.EnvNestedPortPublish)
	}
	if err := tryEnsureSharedMQTTBroker(s); err != nil {
		return err
	}
	material, err := s.store.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		return err
	}
	tlsCfg, err := mqttBridgeTLSConfig(material)
	if err != nil {
		return err
	}
	broker := MQTTBridgeDialAddress()
	if !strings.Contains(broker, "://") {
		broker = "ssl://" + broker
	}
	var (
		mu     sync.Mutex
		client mqtt.Client
	)
	handler := newShadowMQTTHandler(s.store, func(topic string, payload []byte) error {
		mu.Lock()
		c := client
		mu.Unlock()
		if c == nil || !c.IsConnected() {
			return fmt.Errorf("mqtt bridge not connected")
		}
		tok := c.Publish(topic, 0, false, payload)
		if !tok.WaitTimeout(10 * time.Second) {
			return fmt.Errorf("mqtt publish timeout")
		}
		return tok.Error()
	})
	setMQTTRepublish(func(topic string, payload []byte) error {
		mu.Lock()
		c := client
		mu.Unlock()
		if c == nil || !c.IsConnected() {
			return fmt.Errorf("mqtt bridge not connected")
		}
		tok := c.Publish(topic, 0, false, payload)
		if !tok.WaitTimeout(10 * time.Second) {
			return fmt.Errorf("mqtt publish timeout")
		}
		return tok.Error()
	})
	defer setMQTTRepublish(nil)
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetTLSConfig(tlsCfg).
		SetClientID("noctaxris-mqtt-bridge").
		SetAutoReconnect(true)
	opts.OnConnect = func(c mqtt.Client) {
		for _, filter := range ShadowMQTTSubscribeFilters() {
			f := filter
			if token := c.Subscribe(f, 0, func(_ mqtt.Client, msg mqtt.Message) {
				_ = handler.HandleMessage("", msg.Topic(), msg.Payload())
			}); token.Wait() && token.Error() != nil {
				log.Printf("mqtt subscribe %s: %v", f, token.Error())
			}
		}
		// Non-shadow publishes for topic rules (opt-in shared MQTT only).
		if token := c.Subscribe("#", 0, func(_ mqtt.Client, msg mqtt.Message) {
			topic := msg.Topic()
			if strings.HasPrefix(topic, "$aws/") {
				return
			}
			s.DispatchMQTTPublish(topic, msg.Payload())
		}); token.Wait() && token.Error() != nil {
			log.Printf("mqtt subscribe #: %v", token.Error())
		}
		if mqttBridgeRunningHook != nil {
			mqttBridgeRunningHook()
		}
	}
	cli := mqtt.NewClient(opts)
	mu.Lock()
	client = cli
	mu.Unlock()
	token := cli.Connect()
	if !token.WaitTimeout(30 * time.Second) {
		return fmt.Errorf("mqtt bridge connect timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt bridge connect: %w", err)
	}
	<-ctx.Done()
	cli.Disconnect(250)
	return ctx.Err()
}

func mqttBridgeTLSConfig(material store.LabMQTTBrokerMaterial) (*tls.Config, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(material.CACertPEM)) {
		return nil, fmt.Errorf("mqtt bridge: invalid lab CA PEM")
	}
	cert, err := tls.X509KeyPair([]byte(material.BridgeCertPEM), []byte(material.BridgeKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("mqtt bridge tls key pair: %w", err)
	}
	return &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func tryEnsureSharedMQTTBroker(s *Server) error {
	if s == nil {
		return nil
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return fmt.Errorf("shared mqtt: compute client unavailable")
	}
	material, err := s.store.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	inst, err := cli.EnsureDataPlaneByName(ctx, compute.DataPlaneOpts{
		Kind:  compute.DataKindMQTT,
		Image: compute.DefaultDataPlaneImage(compute.DataKindMQTT),
		Name:  compute.LabMQTTContainerName,
		Cmd:   compute.MosquittoStartCmd(material.ContainerConf),
		Binds: material.Binds,
	})
	if err != nil {
		return err
	}
	return cli.WaitDataPlaneHealthy(ctx, inst.ContainerID)
}

// tryEnsureSharedMQTT ensures the Mosquitto singleton when shared MQTT is enabled.
func tryEnsureSharedMQTT(s *Server) error {
	if s == nil || !s.cfg.SharedMQTT {
		return nil
	}
	if !compute.BrokerPortPublishReady() {
		return fmt.Errorf("shared mqtt requires %s or %s", compute.EnvBrokerPortPublish, compute.EnvNestedPortPublish)
	}
	return tryEnsureSharedMQTTBroker(s)
}
