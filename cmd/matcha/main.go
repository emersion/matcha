package main

import (
	"os"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"

	"github.com/emersion/matcha"
)

func main() {
	addr := ":8088"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":"+port
	}

	e := echo.New()
	e.Logger.SetLevel(log.INFO)

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	if err := matcha.New(e, "."); err != nil {
		e.Logger.Fatal(err)
	}

	e.Logger.Fatal(e.Start(addr))
}
