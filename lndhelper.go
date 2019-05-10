// lndhelper
package main

import (
	"io"
	"strings"
	"time"

	"google.golang.org/grpc/status"

	// "google.golang.org/grpc/internal/transport"

	// "github.com/kr/pretty"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc/credentials"

	// "github.com/lightningnetwork/lnd/channeldb"

	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"gopkg.in/macaroon.v2"

	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"sync"
)

type lndHelper struct {
	RhashMu          sync.Mutex
	RhashSettlements map[string](map[string]chan struct{})
	lnClient         lnrpc.LightningClient
	dialOpts         []grpc.DialOption
	connHostPort     string
}

func NewLNDHelper(myConf *conf) *lndHelper {
	helper := &lndHelper{}
	helper.init(myConf)
	return helper
}

func (lh *lndHelper) init(myConf *conf) {
	tlsCreds, err := credentials.NewClientTLSFromFile(myConf.LndTlsCertPath, "")
	// log.Printf("tlsCreds: %# v\n", pretty.Formatter(tlsCreds))
	if err != nil {
		fmt.Println("Cannot get node tls credentials", err)
		return
	}

	macaroonBytes, err := ioutil.ReadFile(myConf.LndMacaroonPath)
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
		// grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(macaroons.NewMacaroonCredential(mac)),
	}
	connHostPort := myConf.LndRpcHostPort
	log.Printf("lndHelper init: about to attempt communication with lnd at %s\n", connHostPort)
	lh.dialOpts = opts
	lh.connHostPort = connHostPort
	lh.lnClient = lh.getClientToLnGrpc()

	log.Printf("lndHelper init: here1\n")

	ctx := context.Background()
	getInfoResp, err := lh.lnClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		fmt.Println("Cannot get info from node:", err)
		return
	}
	log.Printf("lndHelper init: here2\n")

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

func (lh *lndHelper) getClientToLnGrpc() lnrpc.LightningClient {
	conn, err := grpc.Dial(lh.connHostPort, lh.dialOpts...)
	log.Printf("lndHelper init: here0\n")
	// lnConn = conn
	if err != nil {
		log.Printf("lndHelper init: here0.1\n")
		fmt.Println("cannot dial to lnd", err)
		return nil
	}
	log.Printf("lndHelper init: here0.2\n")
	return lnrpc.NewLightningClient(conn)
}

func (lh *lndHelper) NewInvoiceFromLND(sats int64, memo string) *lnrpc.AddInvoiceResponse {
	log.Printf("NewInvoiceFromLND: satoshis: %d, memo: '%s'\n", sats, memo)
	ctx := context.Background()
	//--------------
	// see example in https://github.com/michael1011/lightningtip/blob/master/backends/lnd.go
	// also examples in https://github.com/lightningnetwork/lnd/blob/master/lnd_test.go
	// var invoice *lnrpc.AddInvoiceResponse
	maxLen := 1024 // saving 5mb of built binary size by not importing channeldb for a single value.
	// maxLen := channeldb.MaxMemoSize
	if len(memo) > maxLen {
		//https://bitcoin.stackexchange.com/questions/85951/whats-the-maximum-size-of-the-memo-in-a-ln-payment-request
		//here some guy says 639 chars, but lnd itself seems to use a max of 1024 as defined by that channeldb.MaxMemoSize, so i'm going to go with that.
		memo = memo[:maxLen]
	}

	addInvoiceResp, err := lh.lnClient.AddInvoice(ctx, &lnrpc.Invoice{
		Memo:   memo,
		Value:  sats,
		Expiry: 36000, //3600 is default.
	})

	if err != nil {
		panic(err)
	}
	log.Printf("NewInvoiceFromLND teh AddInvoiceResponse: %#v\n", addInvoiceResp)
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
func (lh *lndHelper) ReadSettled(rhash string, reqId string) chan struct{} {
	lh.RhashMu.Lock()
	if lh.RhashSettlements[rhash] == nil {
		// lh.RhashSettlements[rhash] = make(chan struct{}, 1)
		lh.RhashSettlements[rhash] = map[string]chan struct{}{}
	}
	settledChan := make(chan struct{}, 1)
	lh.RhashSettlements[rhash][reqId] = settledChan
	log.Printf("longPollInvoice: set up a spot for req %s to find out about settlement of %s\n", reqId, rhash)
	lh.RhashMu.Unlock()
	return settledChan
}
func (lh *lndHelper) MonitorInvoices() {
	// lh.RhashSettlements = map[string]chan struct{}{}
	lh.RhashSettlements = map[string](map[string]chan struct{}){}

	for {

		ctx := context.Background()
		log.Printf("MonitorInvoices startup!")
		in := &lnrpc.InvoiceSubscription{}
		subscribeClient, err := lh.lnClient.SubscribeInvoices(ctx, in)
		if err != nil {
			panic(err)
		}
		for {
			invoice, err := subscribeClient.Recv()
			if err == io.EOF {
				log.Printf("MonitorInvoices had EOF err")
				break
			}
			if err != nil {
				log.Printf("?? '%#v' '%#v'", status.Convert(err).Code(), status.Convert(err).Message())
				if strings.Contains(err.Error(), "transport is closing") {
					log.Printf("MonitorInvoices transport is closing")
				}
				if strings.Contains(err.Error(), "all SubConns are in TransientFailure") {
					log.Printf("MonitorInvoices all SubConns are in TransientFailure")
				}
				if strings.Contains(err.Error(), "unknown service lnrpc.Lightning") {
					log.Printf("MonitorInvoices unknown service lnrpc.Lightning (is lnd waiting for wallet unlock??)")
				}
				break
			}

			// if err == transport.ErrConnClosing {
			// 	log.Printf("MonitorInvoices had ErrConnClosing err")
			// 	break
			// }

			if err != nil {
				log.Printf(err.Error())
				panic("stop on account of that unknown error and figure out what to do about it if anything. we should already be handling disconnects.")
			}
			rhash := hex.EncodeToString(invoice.RHash)
			// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(invoice))

			if invoice.State != lnrpc.Invoice_SETTLED {
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
			// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(invoice))
		}

		log.Printf("MonitorInvoices, disconnected, will attempt to reconnect in 10s.")
		time.Sleep(10 * time.Second)
		lh.lnClient = lh.getClientToLnGrpc()
	}
}
func (lh *lndHelper) getInvoiceStatus(rhash string) (settled bool, expired bool) {

	invoice := lh.LookupInvoiceFromLND(rhash)
	settled = (invoice.GetState() == lnrpc.Invoice_SETTLED)
	expired = false
	nowsec := time.Now().UnixNano() / int64(time.Second)
	created := invoice.GetCreationDate()
	expiry := invoice.GetExpiry()
	age := nowsec - created
	expiretime := created + expiry
	if nowsec > expiretime {
		expired = true
	}
	log.Printf("i think time is now %d, invoice creationdate of %d, making it %d seconds old, it has expiry of %d sec aka at %d, so is it expired? %t\n", nowsec, created, age, expiry, expiretime, expired)
	return settled, expired
}
