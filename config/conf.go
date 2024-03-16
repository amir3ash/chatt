package config

type Confing struct {
	MongoHost string `env:"MONGO_HOST" required:"true"`
	MongoUser string `env:"MONGO_USER"`
	MongoPass string `env:"MONGO_PASSWORD"`
	MongoPort int    `env:"MONGO_PORT"`
}

func New() (*Confing, error) {
	conf := &Confing{}
	if err := parse(conf); err != nil {
		return nil, err
	}

	return conf, nil
}
