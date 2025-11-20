package cmd

import (
	"fmt"
	"os"

	"github.com/komari-monitor/komari/cmd/flags"

	"github.com/spf13/cobra"
)

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// 从环境变量获取默认值 - PostgreSQL only
var (
	dbHostEnv = GetEnv("KOMARI_DB_HOST", "localhost")
	dbPortEnv = GetEnv("KOMARI_DB_PORT", "5432")
	dbUserEnv = GetEnv("KOMARI_DB_USER", "komari")
	dbPassEnv = GetEnv("KOMARI_DB_PASS", "")
	dbNameEnv = GetEnv("KOMARI_DB_NAME", "komari")
)

var RootCmd = &cobra.Command{
	Use:   "Komari",
	Short: "Komari is a simple server monitoring tool",
	Long: `Komari is a simple server monitoring tool. 
Made by Akizon77 with love.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SetArgs([]string{"server"})
		cmd.Execute()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// PostgreSQL database configuration
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseHost, "db-host", dbHostEnv, "PostgreSQL host address [env: KOMARI_DB_HOST]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePort, "db-port", dbPortEnv, "PostgreSQL port [env: KOMARI_DB_PORT]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseUser, "db-user", dbUserEnv, "PostgreSQL username [env: KOMARI_DB_USER]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabasePass, "db-pass", dbPassEnv, "PostgreSQL password [env: KOMARI_DB_PASS]")
	RootCmd.PersistentFlags().StringVar(&flags.DatabaseName, "db-name", dbNameEnv, "PostgreSQL database name [env: KOMARI_DB_NAME]")
}
