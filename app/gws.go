package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	kafka "github.com/confluentinc/confluent-kafka-go/kafka"
	logrus "github.com/sirupsen/logrus"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/reports/v1"
	"google.golang.org/api/option"
)

type GWSConfig struct {
	credsFile    string
	subject      string
	applications [21]string
	period       time.Duration
}

func newGWSConfig() *GWSConfig {
	c := &GWSConfig{
		credsFile: "keyfile.json",
		subject:   os.Getenv("SUBJECT"),
		applications: [21]string{
			"access_transparency", "admin", "calendar", "chat", "drive",
			"gcp", "gplus", "groups", "groups_enterprise", "jamboard", "login",
			"meet", "mobile", "rules", "saml", "token", "user_accounts",
			"context_aware_access", "chrome", "data_studio", "keep"},
		period: 60,
	}

	return c
}

type KafkaConfig struct {
	config *kafka.ConfigMap
	topic  string
}

type KafkaEvent struct {
	LogMessage       json.RawMessage `json:"log_message"`
	LogSource        string          `json:"log_source"`
	LogSourcetype    string          `json:"log_sourcetype"`
	LogUtcTimeIngest string          `json:"log_utc_time_ingest"`
}

func newKafkaConfig() *KafkaConfig {
	c := &KafkaConfig{
		config: &kafka.ConfigMap{
			"metadata.broker.list":     os.Getenv("KAFKA_SERVERS"),
			"security.protocol":        "SSL",
			"ssl.ca.location":          "kafka-ca.pem",
			"ssl.certificate.location": "producer-gws-audit.pem",
			"ssl.key.location":         "producer-gws-audit.pem",
		},
		topic: os.Getenv("KAFKA_TOPIC"),
	}

	return c
}

func newKafkaEvent() *KafkaEvent {
	e := &KafkaEvent{
		LogSource: "gws-audit-logs-exporter",
		LogSourcetype: "gws_audit",
	}

	return e
}

type Puller struct {
	gconf   *GWSConfig
	gclient *admin.Service

	kconf *KafkaConfig

	logs chan []byte
	done chan struct{}
}

func pullGWS() {
	p := Puller{
		gconf: newGWSConfig(),

		kconf: newKafkaConfig(),

		logs: make(chan []byte),
		done: make(chan struct{}),
	}

	p.getClient()

	for _, app := range p.gconf.applications {
		go p.getAudit(app)
	}
	go p.sendKafka()

	for {
		<-p.done
	}
}

func (p *Puller) getClient() {
	jsonCredentials, err := os.ReadFile(p.gconf.credsFile)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to read client secret file")
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials, admin.AdminReportsAuditReadonlyScope)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create JWT")
	}
	config.Subject = p.gconf.subject

	ts := config.TokenSource(context.Background())

	p.gclient, err = admin.NewService(context.Background(), option.WithTokenSource(ts))
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create GWS client")
	}
}

func (p *Puller) getAudit(app string) {
	waitTime := p.gconf.period
	start := time.Duration(-waitTime*6) * time.Second
	end := time.Duration(-waitTime*5) * time.Second

	startTime := time.Now().UTC().Add(start).Format(time.RFC3339Nano)
	
	for {
		endTime := time.Now().UTC().Add(end).Format(time.RFC3339Nano)

		r, err := p.gclient.Activities.List("all", app).StartTime(startTime).EndTime(endTime).Do()
		if err != nil {
			logrus.WithError(err).Warn("Unable to retrieve alerts")
			continue
		}

		if len(r.Items) != 0 {
			for _, a := range r.Items {
				_, err := time.Parse(time.RFC3339Nano, a.Id.Time)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"timestamp": a.Id.Time,
					}).WithError(err).Warn("Unable to parse timestamp")
					continue
				}

				be, err := json.Marshal(a)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"alert": a,
					}).WithError(err).Warn("Unable to encode alert to JSON")
					continue
				}

				p.logs <- be
				p.done <- struct{}{}
			}
		}

		time.Sleep(waitTime * time.Second)
		startTime = endTime
	}
}

func (p *Puller) sendKafka() {
	pr, err := kafka.NewProducer(p.kconf.config)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create producer")
	}
	defer pr.Close()

	go func() {
		for e := range pr.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				m := ev
				if m.TopicPartition.Error != nil {
					logrus.WithError(m.TopicPartition.Error).Error("Delivery failed")
					time.Sleep(1 * time.Second)
				} else {
					logrus.WithFields(logrus.Fields{
						"topic":     *m.TopicPartition.Topic,
						"partition": m.TopicPartition.Partition,
						"offset":    m.TopicPartition.Offset,
					}).Info("Delivered message to topic")
				}
			case kafka.Error:
				logrus.WithError(ev).Warn("Generic client error")
			}
		}
	}()

	for {
		e := newKafkaEvent()

		e.LogMessage = <-p.logs
		e.LogUtcTimeIngest = time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")

		enc, _ := json.Marshal(e)

		err = pr.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &p.kconf.topic, Partition: kafka.PartitionAny},
			Value:          enc,
		}, nil)

		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrQueueFull {
				logrus.Warn("Local producer queue is full")
				time.Sleep(5 * time.Second)
				continue
			}
			logrus.WithError(err).Error("Failed to produce message")
			time.Sleep(1 * time.Second)
		}
	}
}

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.WarnLevel)
}

func main() {
	pullGWS()
}
