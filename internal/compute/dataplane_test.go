package compute

import (
	"testing"
)

func TestDataPlaneHostConfigNeverPublishesPorts(t *testing.T) {
	hc := dataPlaneHostConfig()
	if hc.PublishAllPorts {
		t.Fatal("PublishAllPorts must be false")
	}
	if hc.PortBindings != nil && len(hc.PortBindings) > 0 {
		t.Fatalf("PortBindings must be empty, got %#v", hc.PortBindings)
	}
	if string(hc.NetworkMode) != DataPlaneNetworkName {
		t.Fatalf("NetworkMode=%q want %q", hc.NetworkMode, DataPlaneNetworkName)
	}
	if !hostConfigSecurityOK(hc) {
		t.Fatalf("dataplane HostConfig not hardened: Privileged=%v CapAdd=%#v CapDrop=%#v", hc.Privileged, hc.CapAdd, hc.CapDrop)
	}
}

func TestValidateDataPlaneOpts(t *testing.T) {
	t.Run("rejects empty kind", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Image: "postgres:16-alpine"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects unknown kind", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Kind: "mysql", Image: "mysql:8"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects empty image", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{Kind: DataKindRDS})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects negative port", func(t *testing.T) {
		err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindRDS, Image: "postgres:16-alpine", ContainerPort: -1,
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("accepts rds", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindRDS, Image: "postgres:16-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts elasticache", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindElastiCache, Image: "valkey/valkey:8-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts docdb", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindDocDB, Image: "mongo:7",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts mq", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindMQ, Image: "rabbitmq:3.13-alpine",
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepts opensearch", func(t *testing.T) {
		if err := ValidateDataPlaneOpts(DataPlaneOpts{
			Kind: DataKindOpenSearch, Image: "opensearchproject/opensearch:2.11.1",
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStartDataPlaneNilClient(t *testing.T) {
	var c *Client
	_, err := c.StartDataPlane(t.Context(), DataPlaneOpts{
		Kind: DataKindRDS, Image: "postgres:16-alpine",
	})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	if err := c.StopDataPlane(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if _, err := c.InspectDataPlane(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if err := c.WaitDataPlaneHealthy(t.Context(), "x"); err == nil {
		t.Fatal("expected unavailable error")
	}
	if _, err := c.EnsureDataPlaneNetwork(t.Context()); err == nil {
		t.Fatal("expected unavailable error")
	}
}

func TestNestedDataEndpointAndDefaults(t *testing.T) {
	if got := NestedDataEndpoint("noctaxris-data-rds-1", 5432); got != "noctaxris-data-rds-1:5432" {
		t.Fatalf("endpoint=%q", got)
	}
	if DefaultDataPlaneImage(DataKindRDS) != "postgres:16-alpine" {
		t.Fatalf("rds image=%q", DefaultDataPlaneImage(DataKindRDS))
	}
	if DefaultDataPlanePort(DataKindElastiCache) != 6379 {
		t.Fatalf("redis port=%d", DefaultDataPlanePort(DataKindElastiCache))
	}
	if DefaultDataPlanePort(DataKindDocDB) != 27017 {
		t.Fatalf("mongo port=%d", DefaultDataPlanePort(DataKindDocDB))
	}
	if DefaultDataPlanePort(DataKindMQ) != 5672 {
		t.Fatalf("amqp port=%d", DefaultDataPlanePort(DataKindMQ))
	}
	if DefaultDataPlaneImage(DataKindMQ) != "rabbitmq:3.13-alpine" {
		t.Fatalf("mq image=%q", DefaultDataPlaneImage(DataKindMQ))
	}
	if DefaultDataPlanePort(DataKindOpenSearch) != 9200 {
		t.Fatalf("opensearch port=%d", DefaultDataPlanePort(DataKindOpenSearch))
	}
	if DefaultDataPlaneImage(DataKindOpenSearch) != "opensearchproject/opensearch:2.11.1" {
		t.Fatalf("opensearch image=%q", DefaultDataPlaneImage(DataKindOpenSearch))
	}
}

func TestDefaultDataPlaneImageForMQActiveMQ(t *testing.T) {
	if DefaultDataPlaneImageForMQ("RABBITMQ") != defaultRabbitMQImage {
		t.Fatalf("rabbit image=%q", DefaultDataPlaneImageForMQ("RABBITMQ"))
	}
	if DefaultDataPlaneImageForMQ("ACTIVEMQ") != defaultActiveMQImage {
		t.Fatalf("activemq image=%q", DefaultDataPlaneImageForMQ("ACTIVEMQ"))
	}
	if DefaultDataPlaneImageForMQ("activemq") != defaultActiveMQImage {
		t.Fatalf("activemq case fold image=%q", DefaultDataPlaneImageForMQ("activemq"))
	}
	if DefaultDataPlaneImageForMQ("") != defaultRabbitMQImage {
		t.Fatalf("empty engine default image=%q", DefaultDataPlaneImageForMQ(""))
	}
	if defaultActiveMQImage != "apache/activemq-classic:5.18.3" {
		t.Fatalf("pinned ActiveMQ image=%q", defaultActiveMQImage)
	}
}
