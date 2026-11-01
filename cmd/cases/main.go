package main

import (
	"log"
	"os"

	"vkkanban/internal/cases"
)

func main() {
	addr := env("HTTP_ADDR", "127.0.0.1:8094")
	root := env("STATIC_ROOT", "static")
	vue := env("VUE_FILE", "static/board/vue.global.prod.js")
	roomsFile := env("ROOMS_FILE", "data/rooms.json")
	boardFile := env("BOARD_FILE", "data/board.json")
	watchFile := env("UPTIME_FILE", "data/uptime.json")
	err := cases.Serve(addr, map[string]string{
		"/board/":  root + "/board",
		"/rooms/":  root + "/rooms",
		"/uptime/": root + "/uptime",
	}, vue, roomsFile, boardFile, watchFile)
	if err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
