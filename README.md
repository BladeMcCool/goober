# Goober
the goo that bers

## A little more please ??
it is nice to be able to receive money via lightning network on your website. upon browsing the index.html you will get a form that allows you to enter a number of satoshis and a charitable function to earmark the money for. once valid, the form will allow you to generate an invoice from the lnd and display it as a QR code. the page will then poll the backend until the invoice is paid and show a PAID message when complete. It relies on google recaptcha v3 to prevent bot abuse/ddos, and relies on lnd's database of invoices to recall invoice info via rhash and save memo notes. sessions are done via encrypted cookie.

## Prerequisites
a running instance of lnd, which itself requires running a full bitcoin node of some sort (btcd, Bitcoin Core), in my case I am using Bitcoin Core. also site key and secret from recaptcha v3 which is a free anti bot service from google. Goober was written for Go version 1.12.4, which matched the recommended version for the lnd (0.6 i think) I am using. The html page included requires paths for /backend/* to be sent to Goober, a sample nginx configuration is included below.

In order for you to actually be able to get paid your lnd needs to have open incoming channel capacity to it. For example, my node has two channels open to it with remote balance, one opened to me from lnbig (complimentary, how generous of them), and one from bitrefill which was imo a little overpriced. 

## Install
clone the repo, build the code (go build or run the included ./buildrun shellscript), run goober under supervisord or screen or similar to daemonize it. reverse proxy to it from your website. symlink to the web dir from your html doc root so you can go to yoursite/something/index.html and get the web/index.html and its assets.

## What's with that configuration?
the sample config file needs to have proper values put it in and rename to goober.conf.yaml, it looks for it in the current working directory. Goober will listen on port 8081 by default but this can be changed in the config file. You will need to put your recaptcha v3 site key and secret in the config file as well since they are needed for the mandatory built in anti bot protection. you can also allow a regular btc donation option by putting a btc address in the config file onChainBTCAddr setting.

## Anything interesting to know about reverse proxying to goober?
one of the methods is a long poller, designed to go up to 300 sec before timing out and the client will restablish until such time as they close their browser or pay the invoice. keeping the gateway to the backend open for that long required overriding nginx defaults like so:

```
location /backend/ {
    proxy_pass http://localhost:8081/;
    proxy_connect_timeout       300;
    proxy_send_timeout          300;
    proxy_read_timeout          300;
    proxy_set_header            X-Real-IP $remote_addr;
    send_timeout                300;
}
```
also note we add a header to pass the real ip to Goober so that it can be passed along to google for the recaptcha service, otherwise all we will see from nginx for the remoteaddr would be 127.0.0.1. it is not required to add this header but i think it is a good idea to send the real ip to google for recaptcha whenever possible.

## Help, I cant talk to my lnd from an outside server
you might find that your lnd isnt binding to a reachable interface. in that case try putting this in the lnd config (the 0.0.0.0 means bind to all interfaces afaik):
```
rpclisten=0.0.0.0:10009
```
you might also find that the tls certificate doesnt have your external ip in it, in which case goober will just startup and appear to hang with no errors while trying to intiate connection to lnd. I suggest **stopping lnd** (lncli stop), **delete the existing cert and key file** in .lnd dir, and add something like the following to the lnd config, replacing 1.2.3.4 with your actual routable public ip address:
```
tlsextraip=1.2.3.4
```
then **restart lnd** and it should make new cert and key files that include the new ip. i'll note doing it like this should not mess your channels or anything.

## QR code javascript
I'm including qrcode.min.js which is from [KeeeX's fork of qrcodejs](https://github.com/KeeeX/qrcodejs).

## Image assets
Are believed to be public domain. Honk honk.

## Techincal Support
Contact me

## Financial Support
[I'd love some](http://chws.ca/donate)

## License 
[WTFPL](https://choosealicense.com/licenses/wtfpl/)