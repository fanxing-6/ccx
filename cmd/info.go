package cmd

import (
	"ccx/internal"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info <profile>",
	Short: "查看配置详情（从 Gitee Gist 获取）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := internal.LoadAppConfig()
		if err != nil {
			return err
		}

		client := internal.NewGistClientFromConfig(cfg)
		profile, err := client.FetchProfile(name)
		if err != nil {
			return err
		}

		info := internal.ExtractProfileInfo(profile.Settings)
		format := info.APIFormat
		if format == "" {
			format = "anthropic"
		}

		fmt.Printf("配置名称: %s\n", name)
		fmt.Printf("API 格式: %s\n", strings.ToLower(format))
		fmt.Printf("API 地址: %s\n", info.BaseURL)
		if info.Model != "" {
			fmt.Printf("模型:     %s\n", info.Model)
		}
		if info.ReasoningEffort != "" {
			fmt.Printf("思考档位: %s\n", info.ReasoningEffort)
		}
		fmt.Println("\n完整 settings JSON:")
		fmt.Println("─────────────────────")

		formatted, _ := json.MarshalIndent(json.RawMessage(profile.Settings), "", "  ")
		fmt.Println(string(formatted))
		return nil
	},
}
