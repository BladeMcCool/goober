FROM golang:1.12
WORKDIR /go/src/github.com/BladeMcCool
RUN git clone https://github.com/BladeMcCool/goober.git
WORKDIR /go/src/github.com/BladeMcCool/goober
RUN go get -v
RUN apt-get update
RUN apt-get install -y rsync
#so, invalidate cache and get latest (keep all the go get from last time, huge hella build.)
ADD https://api.github.com/repos/BladeMcCool/goober/git/refs/heads/master ./version.json
RUN rm ./version.json
RUN git pull

RUN go build
COPY ./config/* ./ 

EXPOSE 8081
CMD ["goober"]
