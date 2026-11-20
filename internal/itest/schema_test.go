//go:build integration

package itest

import (
	"testing"

	"github.com/huseyinbabal/kafka-ui/internal/kafka"
	"github.com/huseyinbabal/kafka-ui/internal/schemaregistry"
	"github.com/huseyinbabal/kafka-ui/internal/serde"
)

const avroV1 = `{"type":"record","name":"Order","namespace":"it","fields":[{"name":"id","type":"long"},{"name":"status","type":"string"}]}`
const avroV2 = `{"type":"record","name":"Order","namespace":"it","fields":[{"name":"id","type":"long"},{"name":"status","type":"string"},{"name":"note","type":["null","string"],"default":null}]}`
const avroBad = `{"type":"record","name":"Order","namespace":"it","fields":[{"name":"id","type":"string"}]}`

func TestSchemaRegistryRoundTrip(t *testing.T) {
	c := newCluster(t)
	ctx := ctxT(t)
	topic := uniq("it-avro")
	subject := topic + "-value"

	v1, err := c.Schemas.Register(ctx, subject, schemaregistry.RegisterRequest{Schema: avroV1})
	if err != nil || v1.Version != 1 {
		t.Fatalf("register: %+v %v", v1, err)
	}
	check, err := c.Schemas.CheckCompatibility(ctx, subject, schemaregistry.RegisterRequest{Schema: avroBad})
	if err != nil || check.Compatible || len(check.Messages) == 0 {
		t.Fatalf("incompatible schema must be reported with reasons: %+v %v", check, err)
	}
	if _, err := c.Schemas.Register(ctx, subject, schemaregistry.RegisterRequest{Schema: avroBad}); err == nil {
		t.Fatal("registering an incompatible schema must fail")
	}
	if _, err := c.Schemas.Register(ctx, subject, schemaregistry.RegisterRequest{Schema: avroV2}); err != nil {
		t.Fatal(err)
	}
	if err := c.Schemas.SetCompatibility(ctx, subject, "FULL"); err != nil {
		t.Fatal(err)
	}
	d, err := c.Schemas.GetSubject(ctx, subject)
	if err != nil || len(d.Versions) != 2 || d.Compatibility != "FULL" || d.Global == "" {
		t.Fatalf("subject: %+v %v", d, err)
	}
	if err := c.Schemas.SetCompatibility(ctx, subject, ""); err != nil {
		t.Fatal(err)
	}
	subs, err := c.Schemas.ListSubjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range subs {
		found = found || (s.Subject == subject && s.LatestVersion == 2 && s.SchemaType == "AVRO")
	}
	if !found {
		t.Fatalf("subject missing from list: %+v", subs)
	}

	if _, err := kafka.CreateTopic(ctx, c.Admin, kafka.CreateTopicRequest{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := kafka.Produce(ctx, c.Client, c.Admin, c.Serde, topic, kafka.ProduceRequest{
		Value: strp(`{"id": 9007199254740993, "status": "PAID", "note": "rush"}`), ValueFormat: "schema",
		ValueSchema: &serde.SchemaRef{Subject: subject},
	}); err != nil {
		t.Fatal(err)
	}
	s := &sink{}
	if _, err := kafka.Browse(ctx, c.NewClient, c.Admin, kafka.BrowseRequest{Topic: topic, Mode: kafka.SeekOldest, Decoder: c.Decoder()}, s); err != nil {
		t.Fatal(err)
	}
	if len(s.msgs) != 1 || s.msgs[0].Value.Format != serde.Avro ||
		s.msgs[0].Value.Text != `{"id":9007199254740993,"status":"PAID","note":"rush"}` {
		t.Fatalf("decoded %d msgs, value %+v", len(s.msgs), s.msgs[0].Value)
	}

	if _, err := c.Schemas.DeleteSubject(ctx, subject, true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Schemas.GetSubject(ctx, subject); err == nil {
		t.Fatal("subject must be gone after a permanent delete")
	}
}
