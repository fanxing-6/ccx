package cmd

import (
	"ccx/internal"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(defaultCmd)
}

var defaultCmd = &cobra.Command{
	Use:   "default <profile>",
	Short: "设置默认配置",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := internal.LoadAppConfig()
		if err != nil {
			return err
		}

		client := internal.NewGistClientFromConfig(cfg)
		exists, err := client.ProfileExists(name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("配置 %q 在 Gitee Gist 中不存在", name)
		}

		cfg.DefaultProfile = name
		internal.SaveAppConfig(cfg)
		fmt.Printf("默认配置已设置为: %s\n", name)
		return nil
	},
}
