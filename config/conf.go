package config

import (
	"chat-system/core/repo"
	kafkarep "chat-system/core/repo/kafkaRep"
)

type Confing struct {
	MongoDB      *repo.MongoConf
	KafkaWriter  *kafkarep.WriterConf
	KafkaReader  *kafkarep.ReaderConf
	SpiceDbUrl   string `env:"AUTHZED_URL"`
	SpiceDBToken string `env:"AUTHZED_TOKEN"`
}

func New() (*Confing, error) {
	conf := &Confing{}
	if err := Parse(conf); err != nil {
		return nil, err
	}

	return conf, nil
}
