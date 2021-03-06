package tx

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/spf13/cobra"
	amino "github.com/tendermint/go-amino"

	authTypes "github.com/maticnetwork/heimdall/auth/types"
	"github.com/maticnetwork/heimdall/helper"
	"github.com/maticnetwork/heimdall/types/rest"
)

// BroadcastReq defines a tx broadcasting request.
type BroadcastReq struct {
	Tx   authTypes.StdTx `json:"tx"`
	Mode string          `json:"mode"`
}

// BroadcastTxRequest implements a tx broadcasting handler that is responsible
// for broadcasting a valid and signed tx to a full node. The tx can be
// broadcasted via a sync|async|block mechanism.
func BroadcastTxRequest(cliCtx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BroadcastReq

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		err = cliCtx.Codec.UnmarshalJSON(body, &req)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		// check if msg is not nil
		if req.Tx.Msg == nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, errors.New("Invalid msg input").Error())
			return
		}

		// brodcast tx
		res, err := helper.BroadcastTx(cliCtx, req.Tx, req.Mode)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		rest.PostProcessResponse(w, cliCtx, res)
	}
}

// GetBroadcastCommand returns the tx broadcast command.
func GetBroadcastCommand(cdc *amino.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "broadcast [file_path]",
		Short: "Broadcast transactions generated offline",
		Long: strings.TrimSpace(`Broadcast transactions created with the --generate-only
flag and signed with the sign command. Read a transaction from [file_path] and
broadcast it to a node. If you supply a dash (-) argument in place of an input
filename, the command reads from standard input.

$ gaiacli tx broadcast ./mytxn.json
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			cliCtx := context.NewCLIContext().WithCodec(cdc)
			stdTx, err := helper.ReadStdTxFromFile(cliCtx.Codec, args[0])
			if err != nil {
				return
			}

			// brodcast tx
			res, err := helper.BroadcastTx(cliCtx, stdTx, "")
			cliCtx.PrintOutput(res)
			return err
		},
	}

	return client.PostCommands(cmd)[0]
}
