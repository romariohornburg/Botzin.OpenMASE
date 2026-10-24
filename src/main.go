/*
Botzin.OpenMASE
@author: maldoliver
*/
package main

import (
	"fmt"
	"net"
	"os"

	"github.com/chzyer/readline"
	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/progression"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var cfg utils.Config

func main() {
	// Initialize readline for input
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		HistoryFile:     "/tmp/mase.tmp",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		log.Error().Msgf("Error initializing readline: %s", err)
		return
	}
	defer rl.Close()

	// Logs go through readline so they don't break the prompt
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: rl.Stdout()})

	// First and foremost, load our config.
	cfg = utils.GetConfig()

	if len(cfg.XTEA_KEY) != 32 {
		log.Error().Msg("XTEA_KEY must be set to 32 hex characters (see .env.example).")
		os.Exit(1)
	}

	// Load our database
	if err := database.Initialize(cfg.DB_PATH); err != nil {
		os.Exit(1)
	}

	defer database.DB.Close()

	// Seed the item catalog on a fresh database (or reload it after a schema change)
	if _, err := database.LoadItems(cfg.ITEMS_SQL_PATH, database.ItemsSchemaChanged); err != nil {
		log.Warn().Msgf("Could not load items.sql: %s", err)
	}

	if err := progression.RecalculateRanking(); err != nil {
		log.Warn().Msgf("Could not calculate ranking: %s", err)
	}

	// Create the listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.MASE_PORT))

	if err != nil {
		log.Error().Msgf("Listening error: %s", err)
		return
	}

	log.Info().Msgf("MASE - Successfully started listening on port %s.", cfg.MASE_PORT)

	// Close the socket once we're done
	defer listener.Close()

	// HackBuster - has no functionality (for this). Sole purpose is so the client connects and allows rounds to complete.
	go StartHB()
	go StartBuddyList()
	go StartServerList()

	if cfg.REGISTER_PORT != "" {
		go StartRegister()
	}

	go commandInput(rl)

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Error().Msgf("Error thrown whilst accepting connection: %s", err)
			continue
		}

		go handleConnection(conn)
	}
}
