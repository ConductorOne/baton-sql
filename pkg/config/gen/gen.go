package main

import (
	"github.com/conductorone/baton-sdk/pkg/config"
	cfg "github.com/conductorone/baton-sql/pkg/config"
)

func main() {
	config.Generate("sql", cfg.Config)
}
