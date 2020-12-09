# Docker

The docker-compose provides an app container for bee itself and a signer container for Clef.
To prepare your machine to run docker compose execute
```
mkdir -p bee && cd bee
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/docker-compose.yml
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/env -O .env
```
Set all inside `.env`

To configure `clef` set:
- `CLEF_CHAINID=5` for goerli

To configure `bee` set:
- `BEE_CLEF_SIGNER_ENABLE=true` to enable clef support
- `BEE_CLEF_SIGNER_ENDPOINT=http://clef:8550`
- `BEE_SWAP_ENDPOINT=https://rpc.slock.it/goerli`

Set bee password by either setting `BEE_PASSWORD` or `BEE_PASSWORD_FILE`

If you want to use password file set it to
- `BEE_PASSWORD_FILE=/password`

Mount password file local file system by adding
```
- ./password:/password
```
to app volumes inside `docker-compose.yml`

Start it with
```
docker-compose up -d
```

From logs find URL line with `on goerli you can get both goerli eth and goerli bzz from` and prefund your node
```
docker-compose logs -f app
```
