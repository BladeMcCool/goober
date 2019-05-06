// lndhelper
package main

import (
	"encoding/hex"
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"

	"context"
	"fmt"
	"io/ioutil"
	"os/user"
	"path"
	"sync"
)

type lndHelper struct {
	RhashMu          sync.Mutex
	RhashSettlements map[string](map[string]chan struct{})
	lnClient         lnrpc.LightningClient
}

func NewLNDHelper(myConf *conf) *lndHelper {
	helper := &lndHelper{}
	helper.init()
	return helper
}

func (lh *lndHelper) init() {
	usr, err := user.Current()
	if err != nil {
		fmt.Println("Cannot get current user:", err)
		return
	}

	fmt.Println("The user home directory: " + usr.HomeDir)
	tlsCertPath := path.Join(usr.HomeDir, ".lnd/tls.cert")
	macaroonPath := path.Join(usr.HomeDir, ".lnd/data/chain/bitcoin/mainnet/admin.macaroon")
	tlsCreds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		fmt.Println("Cannot get node tls credentials", err)
		return
	}

	macaroonBytes, err := ioutil.ReadFile(macaroonPath)
	if err != nil {
		fmt.Println("Cannot read macaroon file", err)
		return
	}

	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macaroonBytes); err != nil {
		fmt.Println("Cannot unmarshal macaroon", err)
		return
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(macaroons.NewMacaroonCredential(mac)),
	}

	conn, err := grpc.Dial("localhost:10009", opts...)
	// lnConn = conn
	if err != nil {
		fmt.Println("cannot dial to lnd", err)
		return
	}
	lh.lnClient = lnrpc.NewLightningClient(conn)

	ctx := context.Background()
	getInfoResp, err := lh.lnClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		fmt.Println("Cannot get info from node:", err)
		return
	}

	var funBoi = &lnrpc.ListChannelsRequest{}
	getChanResp, err := lh.lnClient.ListChannels(ctx, funBoi)
	if err != nil {
		fmt.Println("Cannot get chan list from node:", err)
		panic("well something seems wrong with comms to lnd.")
	}

	fmt.Printf("%#v \n----\n", []*lnrpc.GetInfoResponse{getInfoResp, getInfoResp})
	spew.Dump(getInfoResp)
	spew.Dump(getChanResp)
	log.Printf("ln client inited and connected successfully")
}

func (lh *lndHelper) GetInvoiceFromLND(sats int64, memo string) *lnrpc.AddInvoiceResponse {
	log.Printf("getInvoiceFromLND: adsats %d, memo %s\n", sats, memo)
	ctx := context.Background()
	//--------------
	// see example in https://github.com/michael1011/lightningtip/blob/master/backends/lnd.go
	// also examples in https://github.com/lightningnetwork/lnd/blob/master/lnd_test.go
	// var invoice *lnrpc.AddInvoiceResponse
	addInvoiceResp, err := lh.lnClient.AddInvoice(ctx, &lnrpc.Invoice{
		Memo:   memo,
		Value:  sats,
		Expiry: 36000, //3600 is default.
	})

	if err != nil {
		panic(err)
	}
	log.Printf("getInvoiceFromLND teh AddInvoiceResponse: %#v\n", addInvoiceResp)
	// return invoice.PaymentRequest
	return addInvoiceResp
}

func (lh *lndHelper) LookupInvoiceFromLND(rhash string) *lnrpc.Invoice {
	ctx := context.Background()
	invoice, _ := lh.lnClient.LookupInvoice(ctx, &lnrpc.PaymentHash{RHashStr: rhash})
	// if err.
	// if err != nil {
	// 	panic(err)
	// }
	if invoice == nil {
		invoice = &lnrpc.Invoice{}
	}
	// log.Printf("lookupInvoiceFromLND: teh invoice: %#v\n", invoice)
	// return invoice.PaymentRequest
	return invoice
}
func (lh *lndHelper) MonitorInvoices() {
	// lh.RhashSettlements = map[string]chan struct{}{}
	lh.RhashSettlements = map[string](map[string]chan struct{}){}
	ctx := context.Background()
	log.Printf("MonitorInvoices startup")
	in := &lnrpc.InvoiceSubscription{}
	subscribeClient, err := lh.lnClient.SubscribeInvoices(ctx, in)
	if err != nil {
		panic(err)
	}
	for {
		wot, err := subscribeClient.Recv()
		rhash := hex.EncodeToString(wot.RHash)
		// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(wot))

		if wot.State != lnrpc.Invoice_SETTLED {
			continue
		}
		log.Printf("MonitorInvoices: got invoice settlement for %s\n", rhash)

		// if lh.RhashSettlements[rhash] == nil || len(lh.RhashSettlements[rhash]) == 0 {
		// 	//a long polling channel reader would have created the channel about the invoice it was interested in knowing was paid. no channel, no interest.
		// 	continue
		// }
		if lh.RhashSettlements[rhash] == nil {
			log.Printf("MonitorInvoices: there is nothing defined under rhash settlements map for %s\n", rhash)
			//a long polling channel reader would have created the channel about the invoice it was interested in knowing was paid. no channel, no interest.
			continue
		}
		// if len(lh.RhashSettlements[rhash]) > 0 {
		// 	//hrm i dont think this can actually happen.
		// 	continue
		// }
		// lh.RhashSettlements[rhash] = make(chan struct{}, 1)
		log.Printf("MonitorInvoices: there are %d requests wanting to know about settlement of %s\n", len(lh.RhashSettlements[rhash]), rhash)
		for reqId := range lh.RhashSettlements[rhash] {
			lh.RhashSettlements[rhash][reqId] <- struct{}{}
			log.Printf("MonitorInvoices: so, umm, we just put something in the channel for reqid %s to know about settlement of %s\n", reqId, rhash)
		}
		if err != nil {
			panic(err)
		}
		// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(wot))
	}

}
