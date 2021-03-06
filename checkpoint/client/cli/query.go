package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/maticnetwork/heimdall/checkpoint/types"
	hmClient "github.com/maticnetwork/heimdall/client"
)

// GetQueryCmd returns the cli query commands for this module
func GetQueryCmd(cdc *codec.Codec) *cobra.Command {
	// Group supply queries under a subcommand
	supplyQueryCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Querying commands for the checkpoint module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       hmClient.ValidateCmd,
	}

	// supply query command
	supplyQueryCmd.AddCommand(
		client.GetCommands(
			GetCheckpointBuffer(cdc),
			GetLastNoACK(cdc),
			GetHeaderFromIndex(cdc),
			GetCheckpointCount(cdc),
		)...,
	)

	return supplyQueryCmd
}

// GetCheckpointBuffer get checkpoint present in buffer
func GetCheckpointBuffer(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint-buffer",
		Short: "show checkpoint present in buffer",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			res, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.QuerierRoute, types.QueryCheckpointBuffer), nil)
			if err != nil {
				return err
			}

			if len(res) == 0 {
				return errors.New("No checkpoint buffer found")
			}

			fmt.Printf(string(res))
			return nil
		},
	}

	return cmd
}

// GetLastNoACK get last no ack time
func GetLastNoACK(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "last-noack",
		Short: "get last no ack received time",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			res, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.QuerierRoute, types.QueryLastNoAck), nil)
			if err != nil {
				return err
			}

			if len(res) == 0 {
				return errors.New("No last-no-ack count found")
			}

			var lastNoAck uint64
			if err := cliCtx.Codec.UnmarshalJSON(res, &lastNoAck); err != nil {
				return err
			}

			fmt.Printf("LastNoACK received at %v", time.Unix(int64(lastNoAck), 0))
			return nil
		},
	}

	return cmd
}

// GetHeaderFromIndex get checkpoint given header index
func GetHeaderFromIndex(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "header",
		Short: "get checkpoint (header) from index",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)
			headerNumber := viper.GetInt(FlagHeaderNumber)

			// get query params
			queryParams, err := cliCtx.Codec.MarshalJSON(types.NewQueryCheckpointParams(uint64(headerNumber)))
			if err != nil {
				return err
			}

			// fetch checkpoint
			res, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.QuerierRoute, types.QueryCheckpoint), queryParams)
			if err != nil {
				return err
			}

			fmt.Printf(string(res))
			return nil
		},
	}
	cmd.MarkFlagRequired(FlagHeaderNumber)

	return cmd
}

// GetCheckpointCount get number of checkpoint received count
func GetCheckpointCount(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint-count",
		Short: "get checkpoint counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			res, _, err := cliCtx.QueryWithData(fmt.Sprintf("custom/%s/%s", types.QuerierRoute, types.QueryAckCount), nil)
			if err != nil {
				return err
			}

			if len(res) == 0 {
				return errors.New("No ack count found")
			}

			var ackCount uint64
			if err := cliCtx.Codec.UnmarshalJSON(res, &ackCount); err != nil {
				return err
			}

			fmt.Printf("Total number of checkpoint so far : %v", ackCount)
			return nil
		},
	}

	return cmd
}
