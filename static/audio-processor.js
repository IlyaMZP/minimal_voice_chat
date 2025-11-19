class CircularBuffer {
  constructor(capacity) {
    this.capacity = capacity;
    this.buffer = new Float32Array(capacity);
    this.readPosition = 0;
    this.writePosition = 0;
    this.availableSamples = 0;
  }

  isEmpty() {
    return this.availableSamples === 0;
  }

  isFull() {
    return this.availableSamples === this.capacity;
  }

  enqueue(data) {
    const dataLength = data.length;

    if (this.availableSamples + dataLength > this.capacity) {
      // Not enough space, overwrite old data
      const overflow = (this.availableSamples + dataLength) - this.capacity;
      this.readPosition = (this.readPosition + overflow) % this.capacity;
      this.availableSamples -= overflow;
    }

    // Write data to buffer
    for (let i = 0; i < dataLength; i++) {
      this.buffer[this.writePosition] = data[i];
      this.writePosition = (this.writePosition + 1) % this.capacity;
    }

    this.availableSamples += dataLength;
  }

  dequeue(numSamples) {
    if (this.isEmpty() || numSamples <= 0) {
      return new Float32Array(0);
    }

    const samplesToRead = Math.min(numSamples, this.availableSamples);
    const result = new Float32Array(samplesToRead);

    for (let i = 0; i < samplesToRead; i++) {
      result[i] = this.buffer[this.readPosition];
      this.readPosition = (this.readPosition + 1) % this.capacity;
    }

    this.availableSamples -= samplesToRead;
    return result;
  }

  peek(numSamples) {
    if (this.isEmpty() || numSamples <= 0) {
      return new Float32Array(0);
    }

    const samplesToRead = Math.min(numSamples, this.availableSamples);
    const result = new Float32Array(samplesToRead);

    let tempReadPos = this.readPosition;
    for (let i = 0; i < samplesToRead; i++) {
      result[i] = this.buffer[tempReadPos];
      tempReadPos = (tempReadPos + 1) % this.capacity;
    }

    return result;
  }

  clear() {
    this.readPosition = 0;
    this.writePosition = 0;
    this.availableSamples = 0;
  }

  getAvailableSamples() {
    return this.availableSamples;
  }
}

class AudioMixerProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super(options);

    // Map to store circular buffers for each channel
    this.channelBuffers = new Map();
    this.BUFFER_SIZE = Math.ceil(1024*sampleRate/16000);
    console.log(this.BUFFER_SIZE)

    this.inputBuffer = new Float32Array(this.BUFFER_SIZE);
    this.inputBufferPtr = 0;

    this.bufferCapacity = this.BUFFER_SIZE*4; // 4x the typical chunk size around 256ms.

    this.port.onmessage = (event) => {
      const { channel, data } = event.data;

      if (!this.channelBuffers.has(channel)) {
        this.channelBuffers.set(channel, new CircularBuffer(this.bufferCapacity));
      }

      const channelBuffer = this.channelBuffers.get(channel);
      channelBuffer.enqueue(data);
    };
  }

  process(inputs, outputs, parameters) {
    const output = outputs[0];
    const input = inputs[0];
    if (input && input.length !== 0 || input[0].length !== 0) {
        const channelData = input[0];
        let offset = 0;
        while (offset < channelData.length) {
          const spaceRemaining = this.inputBuffer.length - this.inputBufferPtr;
          const copyCount = Math.min(spaceRemaining, channelData.length - offset);
          // Copy chunk
          this.inputBuffer.set(channelData.subarray(offset, offset + copyCount), this.inputBufferPtr);
          this.inputBufferPtr += copyCount;
          offset += copyCount;
          if (this.inputBufferPtr >= this.inputBuffer.length) {
            // Copy before sending, since Float32Array is reused
            this.port.postMessage(this.inputBuffer.slice(0));
            this.inputBufferPtr = 0;
          }
        }
    }

    if (output.length === 0) return true;

    const samplesNeeded = output[0].length; // Typically 128, but the quant size might change in the future

    const outputBuffer = new Float32Array(samplesNeeded).fill(0);

    // If no channels have data, output silence
    if (this.channelBuffers.size === 0) {
      output.forEach((channel) => {
        channel.set(outputBuffer);
      });
      return true;
    }

    let activeChannels = 0;
    let maxAmplitude = 0;

    // Mix all channels
    for (const [channelId, channelBuffer] of this.channelBuffers) {
      if (!channelBuffer.isEmpty()) {
        const channelData = channelBuffer.dequeue(samplesNeeded);

        if (channelData.length > 0) {
          activeChannels++;

          // Sum the channel data into output buffer
          for (let i = 0; i < channelData.length; i++) {
            outputBuffer[i] += channelData[i];
          }

          // Update max amplitude for normalization
          for (let i = 0; i < channelData.length; i++) {
            maxAmplitude = Math.max(maxAmplitude, Math.abs(outputBuffer[i]));
          }
        }
      }
    }

    // Normalize to avoid clipping if necessary
    if (maxAmplitude > 1.0) {
      for (let i = 0; i < outputBuffer.length; i++) {
        outputBuffer[i] /= maxAmplitude;
      }
    }

    // Output the mono audio to all channels
    output.forEach((channel) => {
      channel.set(outputBuffer);
    });

    return true;
  }
}

registerProcessor('audio-mixer-processor', AudioMixerProcessor);
