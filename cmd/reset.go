package cmd

import (
	"ccx/internal"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(resetCmd)
}

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "重置本地 Gitee 连接配置（删除 ~/.config/ccx/config.json）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !internal.ConfirmAction("确认删除本地配置文件 ~/.config/ccx/config.json") {
			fmt.Println("已取消")
			return nil
		}
		if err := internal.DeleteAppConfig(); err != nil {
			return err
		}
		fmt.Println("已删除本地配置文件。下次运行 ccx 会提示重新初始化（ccx init）。")
		return nil
	},
}
