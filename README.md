# Goober
the goo that bers


## A little more please ??
it is nice to be able to receive money via lightning network on your website. upon browsing the index.html you will get a form that allows you to enter a number of satoshis and a charitable function to earmark the money for. once valid, the form will allow you to generate an invoice from the lnd and display it as a QR code. the page will then poll the backend until the invoice is paid and show a PAID message when complete. It relies on google recaptcha v3 to prevent bot abuse/ddos, and relies on lnd's database of invoices to recall invoice info via rhash and save memo notes.


## Install
clone the repo, build the code (golang), run goober under supervisord or screen or something to daemonize it. reverse proxy to it from your website. symlink to the web dir from your html doc root so you can go to yoursite/something/index.html and get the web/index.html and its assets.

## What's with that configuration?
the sample config file needs to have proper values put it in and rename to goober.conf.yaml, it looks for it in the current working directory. goober is presently hardcoded to run on port 8081. also **you must edit the index.html** and replace both occurrences of 6LdilaAUAAAAAC5xBxHpoaZ7h3gqEXsTZdY0nkfc with your actual recaptcha v3 site key. (TODO: put this in the config as well and add a backend method to return this to the page to prep the recaptcha script load.)

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

also note we add the real ip so the code can pick it up and use in the captcha data sent to google

## Help, I cant talk to my lnd from an outside server
you might find that your lnd isnt binding to a reachable interface. in that case try putting this in the lnd config (the 0.0.0.0 means bind to all interfaces afaik):
```
rpclisten=0.0.0.0:10009
```
you might also find that the tls certificate doesnt have your external ip in it, in which case goober will just startup and appear to hang with no errors while trying to intiate connection to lnd. I suggest **stopping lnd** (lncli stop), **delete the existing cert and key file** in .lnd dir, and add something like the following to the lnd config, replacing 1.2.3.4 with your actual routable public ip address:
```
tlsextraip=1.2.3.4
```

then **restart lnd** and it should make new cert and key files that include the new ip. I have goober running under supervisord to daemonize it.

## Prerequisites
a running instance of lnd obviously. which needs to talk to a full indexing node, in my case I am using Bitcoin Core (b/c I dont think neutrino support is ready, is it??). this was written for Go version 1.12.4, which matched the recommended version for the lnd (0.6 i think) I am using. in order for you to actually be able to get paid your lnd needs to have open incoming channel capacity to it. For example, my node has two channels open to it with remote balance, one opened to me from lnbig (complimentary, how generous of them), and one from bitrefill which was imo a little overpriced. 

## QR code javascript
I'm including qrcode.min.js which is from [KeeeX's fork of qrcodejs](https://raw.githubusercontent.com/KeeeX/qrcodejs/1c87e7fbee2da04ae6c404ad13f9522ea8c9120c/qrcode.min.js), because it works properly.

## Image assets
Are believed to be public domain. Honk honk.

## Support
Contact me for rates

## License 
[WTFPL](https://choosealicense.com/licenses/wtfpl/)