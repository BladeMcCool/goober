#!/bin/bash
set -e
scp -C -P 10022 goober goober@chws.ca:/shared/goober/
#scp -C -P 10022 -r web goober@chws.ca:/shared/goober/
rsync -avz -e "ssh -p 10022" web goober@chws.ca:/shared/goober/
