package cmd

import (
	"github.com/janos/bee"

	"github.com/spf13/cobra"
)

func (c *command) initVersionCmd() {
	c.root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version number",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(bee.Version)
		},
	})
}
