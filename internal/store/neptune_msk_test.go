package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestNeptuneClusterCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	c, err := st.CreateNeptuneCluster(account, "us-east-1", "lab-neptune-1", "neptune", "1.3.0.0", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "creating" || c.EndpointPort != 8182 {
		t.Fatalf("got %+v", c)
	}
	if c.GraphEngine != store.NeptuneGraphEngineGremlin {
		t.Fatalf("graphEngine=%q", c.GraphEngine)
	}
	if c.EndpointAddress != "lab-neptune-1.neptune.noctaxris.internal" {
		t.Fatalf("endpoint=%q", c.EndpointAddress)
	}
	if _, err := st.CreateNeptuneCluster(account, "us-east-1", "lab-neptune-1", "neptune", "", "", 0); !errors.Is(err, store.ErrNeptuneClusterExists) {
		t.Fatalf("exists? err=%v", err)
	}
	if _, err := st.CreateNeptuneCluster(account, "us-east-1", "x", "docdb", "", "", 0); !errors.Is(err, store.ErrNeptuneBadRequest) {
		t.Fatalf("docdb rejected? err=%v", err)
	}
	if err := st.SetNeptuneContainerID(account, "lab-neptune-1", "ctr-1", "available", ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeNeptuneCluster(account, "lab-neptune-1")
	if err != nil || got.Status != "available" || got.ContainerID != "ctr-1" {
		t.Fatalf("describe %+v err=%v", got, err)
	}
	if got.EndpointAddress != "lab-neptune-1.neptune.noctaxris.internal" {
		t.Fatalf("promote must keep DNS endpoint, got %q", got.EndpointAddress)
	}
	list, err := st.DescribeNeptuneClusters(account, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	cid, err := st.DeleteNeptuneCluster(account, "lab-neptune-1")
	if err != nil || cid != "ctr-1" {
		t.Fatalf("delete cid=%q err=%v", cid, err)
	}
	if _, err := st.DescribeNeptuneCluster(account, "lab-neptune-1"); !errors.Is(err, store.ErrNeptuneClusterNotFound) {
		t.Fatalf("after delete err=%v", err)
	}
}

func TestNeptuneGraphEngineSelection(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	t.Run("default gremlin", func(t *testing.T) {
		c, err := st.CreateNeptuneCluster(account, "us-east-1", "lab-gremlin", "neptune", "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if c.GraphEngine != store.NeptuneGraphEngineGremlin || c.EndpointPort != store.NeptuneNestedPort {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("neo4j bolt port", func(t *testing.T) {
		c, err := st.CreateNeptuneCluster(account, "us-east-1", "lab-neo4j", "neptune", "", store.NeptuneGraphEngineNeo4j, 0)
		if err != nil {
			t.Fatal(err)
		}
		if c.GraphEngine != store.NeptuneGraphEngineNeo4j || c.EndpointPort != store.NeptuneNestedBoltPort {
			t.Fatalf("got %+v", c)
		}
		got, err := st.DescribeNeptuneCluster(account, "lab-neo4j")
		if err != nil || got.GraphEngine != store.NeptuneGraphEngineNeo4j || got.EndpointPort != 7687 {
			t.Fatalf("describe %+v err=%v", got, err)
		}
	})

	t.Run("reject unknown graph engine", func(t *testing.T) {
		_, err := st.CreateNeptuneCluster(account, "us-east-1", "lab-bad", "neptune", "", "arangodb", 0)
		if !errors.Is(err, store.ErrNeptuneBadRequest) {
			t.Fatalf("want bad request, got %v", err)
		}
	})

	t.Run("normalize aliases", func(t *testing.T) {
		ge, err := store.NormalizeNeptuneGraphEngine("openCypher")
		if err != nil || ge != store.NeptuneGraphEngineNeo4j {
			t.Fatalf("ge=%q err=%v", ge, err)
		}
		ge, err = store.NormalizeNeptuneGraphEngine("tinkerpop")
		if err != nil || ge != store.NeptuneGraphEngineGremlin {
			t.Fatalf("ge=%q err=%v", ge, err)
		}
	})

	t.Run("env fail-closed", func(t *testing.T) {
		t.Setenv(store.EnvNeptuneEngine, "not-a-backend")
		_, err := store.NeptuneGraphEngineFromEnv()
		if !errors.Is(err, store.ErrNeptuneBadRequest) {
			t.Fatalf("want bad request, got %v", err)
		}
		t.Setenv(store.EnvNeptuneEngine, "neo4j")
		ge, err := store.NeptuneGraphEngineFromEnv()
		if err != nil || ge != store.NeptuneGraphEngineNeo4j {
			t.Fatalf("ge=%q err=%v", ge, err)
		}
		t.Setenv(store.EnvNeptuneEngine, "")
		ge, err = store.NeptuneGraphEngineFromEnv()
		if err != nil || ge != store.NeptuneGraphEngineGremlin {
			t.Fatalf("empty env default ge=%q err=%v", ge, err)
		}
	})
}

func TestMSKClusterCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	c, err := st.CreateMSKCluster(account, "us-east-1", "lab-msk-1", "3.6.0", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != store.MSKClusterStateCreating {
		t.Fatalf("state=%q", c.State)
	}
	if c.BootstrapBrokers != "noctaxris-msk-lab-msk-1:9092" {
		t.Fatalf("bootstrap=%q", c.BootstrapBrokers)
	}
	if _, err := st.CreateMSKCluster(account, "us-east-1", "lab-msk-1", "", 1, false); !errors.Is(err, store.ErrMSKClusterExists) {
		t.Fatalf("exists? err=%v", err)
	}
	if err := st.SetMSKContainerID(account, "lab-msk-1", "ctr-msk", store.MSKClusterStateActive, "noctaxris-msk-lab-msk-1:9092"); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeMSKClusterByARN(account, c.ClusterARN)
	if err != nil || got.State != store.MSKClusterStateActive || got.ContainerID != "ctr-msk" {
		t.Fatalf("describe %+v err=%v", got, err)
	}
	list, err := st.ListMSKClusters(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	cid, err := st.DeleteMSKCluster(account, c.ClusterARN)
	if err != nil || cid != "ctr-msk" {
		t.Fatalf("delete cid=%q err=%v", cid, err)
	}
}
