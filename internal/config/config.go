// Пакет config загружает конфигурацию приложения из YAML-файла.
// Реализуйте этот пакет самостоятельно.
package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// Config содержит параметры запуска сервера.
// Изучите config.yaml и добавьте поля самостоятельно.
type Config struct {
	ServerHost        string `yaml:"server_host"`
	ServerPort        int    `yaml:"server_port"`
	LogLevel          string `yaml:"log_level"`
	AccrualInterval   int    `yaml:"accrual_interval_seconds"`
	WorkerConcurrency int    `yaml:"worker_concurrency"`
}

// Load читает конфигурацию из файла config.yaml.
// Если файл не найден или поле не задано, применяются значения по умолчанию.
func Load() (*Config, error) {
	config := &Config{
		ServerHost:        "localhost",
		ServerPort:        8080,
		LogLevel:          "info",
		AccrualInterval:   3,
		WorkerConcurrency: 5,
	}

	yamlFile, err := os.ReadFile("config.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		log.Printf("предупреждение: не удалось прочитать config.yaml: %v, используются значения по умолчанию", err)
		return config, nil
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Printf("предупреждение: ошибка парсинга config.yaml: %v, используются значения по умолчанию", err)
		return config, nil
	}

	return config, nil
}
