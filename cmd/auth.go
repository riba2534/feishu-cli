package cmd

import "github.com/spf13/cobra"

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "认证管理命令",
}

func init() {
	rootCmd.AddCommand(authCmd)
}
