#!/bin/bash
clang --target=wasm32 -O3 -nostdlib -Wl,--no-entry -Wl,--export-memory -o audio.wasm audio.c
