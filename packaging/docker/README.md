# Docker compose

The docker-compose provides an app container for Bee itself and a signer container for Clef.
To prepare your machine to run docker compose execute
```
mkdir -p bee && cd bee
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/docker-compose.yml
wget -q https://raw.githubusercontent.com/ethersphere/bee/master/packaging/docker/env -O .env
```
Set all configuration variables inside `.env`

`clef` is configured with `CLEF_CHAINID=5` for goerli

To configure `bee` set obtain a free Infura account and set:
- `BEE_SWAP_ENDPOINT=wss://goerli.infura.io/ws/v3/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

Set bee password by either setting `BEE_PASSWORD` or `BEE_PASSWORD_FILE`

If you want to use password file set it to
- `BEE_PASSWORD_FILE=/password`

Mount password file local file system by adding
```
- ./password:/password
```
to bee volumes inside `docker-compose.yml`

Start it with
```
docker-compose up -d
```

From logs find URL line with `on goerli you can get both goerli eth and goerli bzz from` and prefund your node
```
docker-compose logs -f bee-1
```

Update services with
```
docker-compose pull && docker-compose up -d
```

## Running multiple Bee nodes
It is easy to run multiple bee nodes with docker compose by adding more services to `docker-compose.yaml`
To do so, open `docker-compose.yaml`, copy lines 3-58 and past this after line 58.
In the copied lines, replace all occurences of `bee-1` with `bee-2`, `clef-1` with `clef-2` and adjust the `API_ADDR` and `P2P_ADDR` and `DEBUG_API_ADDR` to respectively `1733`, `1734` and `127.0.0.1:1735`
Lastly, add your newly configured services under `volumes` (last lines), such that it looks like:
```yaml
volumes:
  clef-1:
  bee-1:
  bee-2:
  clef-2:
```

If you want to create more than two nodes, simply repeat the process above, ensuring that you keep unique name for your bee and clef services and update the ports