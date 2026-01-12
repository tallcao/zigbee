package config

import (
	"log"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	DBHost     string `mapstructure:"DB_HOST"`
	DBPort     string `mapstructure:"DB_PORT"`
	DBName     string `mapstructure:"DB_NAME"`
	DBUser     string `mapstructure:"DB_USER"`
	DBPassword string `mapstructure:"DB_PASSWORD"`

	MqttBroker   string `mapstructure:"MQTT_BROKER"`
	MqttUser     string `mapstructure:"MQTT_USER"`
	MqttPassword string `mapstructure:"MQTT_PASSWORD"`

	NatsUrl string `mapstructure:"NATS_URL"`
}

func LoadConfig() (Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("Error reading config file, using environment variables: %s", err)
		// If .env file is not found, try to read from environment variables directly
		var config Config
		config.DBHost = os.Getenv("DB_HOST")
		config.DBUser = os.Getenv("DB_USER")
		config.DBPassword = os.Getenv("DB_PASSWORD")
		config.DBName = os.Getenv("DB_NAME")
		config.DBPort = os.Getenv("DB_PORT")

		config.MqttBroker = os.Getenv("MQTT_BROKER")
		config.MqttUser = os.Getenv("MQTT_USER")
		config.MqttPassword = os.Getenv("MQTT_PASSWORD")

		config.NatsUrl = os.Getenv("NATS_URL")

		// Check if required environment variables are set
		if config.DBHost == "" || config.DBUser == "" || config.DBPassword == "" || config.DBName == "" || config.DBPort == "" || config.MqttBroker == "" {
			log.Println("Required environment variables are not set")
			return config, err
		}
		return config, nil
	}

	var config Config
	err = viper.Unmarshal(&config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}
