package config

type Confing struct {
	MongoHost    string `env:"MONGO_HOST" required:"true"`
	MongoUser    string `env:"MONGO_USER" required:"true"`
	MongoPass    string `env:"MONGO_PASSWORD"`
	MongoPort    int    `env:"MONGO_PORT"`
	SpiceDbUrl   string `env:"AUTHZED_URL"`
	SpiceDBToken string `env:"AUTHZED_TOKEN"`
	KafkaHost    string `env:"KAFKA_HOST"`
}

func New() (*Confing, error) {
	conf := &Confing{}
	if err := parse(conf); err != nil {
		return nil, err
	}

	if conf.MongoPort == 0 {
		conf.MongoPort = 27017
	}

	return conf, nil
}
