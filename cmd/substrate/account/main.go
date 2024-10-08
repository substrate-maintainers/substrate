package account

import (
	"github.com/spf13/cobra"
	"github.com/src-bin/substrate/cmd/substrate/account/adopt"
	"github.com/src-bin/substrate/cmd/substrate/account/close"
	"github.com/src-bin/substrate/cmd/substrate/account/create"
	"github.com/src-bin/substrate/cmd/substrate/account/list"
	"github.com/src-bin/substrate/cmd/substrate/account/update"
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "manage AWS accounts",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(adopt.Command())
	cmd.AddCommand(close.Command())
	cmd.AddCommand(create.Command())
	cmd.AddCommand(list.Command())
	cmd.AddCommand(update.Command())

	return cmd
}
