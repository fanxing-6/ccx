package cmd

import (
	"ccx/internal"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "列出所有配置（从 Gitee Gist 获取）",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := internal.LoadAppConfig()
		if err != nil {
			return err
		}

		client := internal.NewGistClientFromConfig(cfg)
		fmt.Println("正在从 Gitee Gist 获取配置列表...")
		profiles, err := client.FetchAllProfiles()
		if err != nil {
			return err
		}

		if len(profiles) == 0 {
			fmt.Println("Gist 中没有配置")
			return nil
		}

		defaultProfile := cfg.DefaultProfile

		fmt.Printf("\n共 %d 个配置：\n\n", len(profiles))
		for _, p := range profiles {
			marker := "  "
			if p.Name == defaultProfile {
				marker = "* "
			}

			info := internal.ExtractProfileInfo(p.Settings)
			model := ""
			if info.Model != "" {
				model = fmt.Sprintf("  [%s]", info.Model)
			}
			url := ""
			if info.BaseURL != "" {
				url = fmt.Sprintf("  %s", info.BaseURL)
			}
			fmt.Printf("%s%-20s%s%s\n", marker, p.Name, url, model)
		}
		fmt.Println("\n* 表示默认配置")
		return nil
	},
}
