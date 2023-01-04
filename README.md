## Google Workspace Audit Collector

A simple application for collecting audit logs from your Google Workspace account and producing the events to Kafka topic. The app is designed to work continuously inside a container.

Collection of events generated in a 60-second sliding time window from 6 to 5 minutes ago is triggered each 60 seconds. The scheme guarantees that Workspace has time to generate all audit events for all actions in the account and will be available using the API.

### Audit scope

The app collects traces of all activity inside Google Workspace account using Google Workspace Admin Console through [Reports API](https://developers.google.com/admin-sdk/reports) provided by all the internal applications: `access_transparency`, `admin`, `calendar`, `chat`, `drive`, `gcp`, `gplus`, `groups`, `groups_enterprise`, `jamboard`, `login`, `meet`, `mobile`, `rules`, `saml`, `token`, `user_accounts`, `context_aware_access`, `chrome`, `data_studio`, `keep`.

### Usage

Declare environment variables and run:

```
docker build --tag gws-audit . \
  && docker run --rm \
       -e SUBJECT=$SUBJECT \
       -e KAFKA_SERVERS=$KAFKA_SERVERS \
       -e KAFKA_TOPIC=$KAFKA_TOPIC \
       --name gws-audit \
       gws-audit
```

Variables:
- `SUBJECT` is the principal name of any account that has access to the apropriate service account
- `KAFKA_SERVERS` is an initial list of brokers as a CSV list of brokers in format `host1:9092,host2:9092,host3:9092`
- `KAFKA_TOPIC` is a name of Kafka topic

Files:
- `app/keyfile.json` contains Google Cloud [service account key](https://cloud.google.com/iam/docs/creating-managing-service-account-keys) and must be stored securely!
- `app/kafka-ca.pem` contains Kafka CA certificate for verifying the broker's key
- `app/producer-gws-audit.pem` contains Kafka client's public and private keys


### Authentication

Reports API only accepts [OAuth 2.0 for Service Accounts](https://developers.google.com/identity/protocols/oauth2/service-account).

Scope: https://www.googleapis.com/auth/admin.reports.audit.readonly
