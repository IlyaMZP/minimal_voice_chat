# minimal_voice_chat
Not so minimal voice chat using websocket, wasm and AuidioWorklet

## Set-up
Put the contents of the `static` directoy on your http server\
Configure your http server to proxy the websocket connection on /vc/socket to localhost:8765 or change the websocket url in the `index.html`\
Launch the websocket server:
```bash
$ go run server.go
```
Navigate to the `index.html` page in your browser and click `Start Chat`.

G711 implementation: https://github.com/dystopiancode/pcm-g711/
Resampler: https://github.com/cpuimage/resampler/
