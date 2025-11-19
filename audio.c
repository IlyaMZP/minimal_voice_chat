#include <stdint.h>
#include <stddef.h>

int8_t MuLaw_Encode(int16_t number)
{
	const uint16_t MULAW_MAX = 0x1FFF;
	const uint16_t MULAW_BIAS = 33;
	uint16_t mask = 0x1000;
	uint8_t sign = 0;
	uint8_t position = 12;
	uint8_t lsb = 0;
	if (number < 0)
	{
		number = -number;
		sign = 0x80;
	}
	number += MULAW_BIAS;
	if (number > MULAW_MAX)
	{
		number = MULAW_MAX;
	}
	for (; ((number & mask) != mask && position >= 5); mask >>= 1, position--)
		;
	lsb = (number >> (position - 4)) & 0x0f;
	return (~(sign | ((position - 5) << 4) | lsb));
}

int16_t MuLaw_Decode(int8_t number)
{
	const uint16_t MULAW_BIAS = 33;
	uint8_t sign = 0, position = 0;
	int16_t decoded = 0;
	number = ~number;
	if (number & 0x80)
	{
		number &= ~(1 << 7);
		sign = -1;
	}
	position = ((number & 0xF0) >> 4) + 5;
	decoded = ((1 << position) | ((number & 0x0F) << (position - 4))
			| (1 << (position - 5))) - MULAW_BIAS;
	return (sign == 0) ? (decoded) : (-(decoded));
}

static int16_t float_to_int16(float sample) {
    // 1. Clamp the sample to the valid range [-1.0, 1.0]
    if (sample > 1.0f) {
        sample = 1.0f;
    } else if (sample < -1.0f) {
        sample = -1.0f;
    }

    // 2. Scale to the 14-bit range required by mu-law
    if (sample < 0.0f) {
        return (int16_t)(sample * 8192.0f); // Map [-1.0, 0) to [-8192, 0)
    } else {
        return (int16_t)(sample * 8191.0f); // Map [0, 1.0] to [0, 8191]
    }
}

static float int16_to_float(int16_t sample) {
    // Convert signed int16 (14b) to the range [-1.0, 1.0]
    return (sample >= 0)
        ? (sample / 8191.0f)
        : (sample / 8192.0f);
}


__attribute__((export_name("encode_audio")))
void encode_audio(float* input_ptr, int8_t* output_ptr, int count) {
  for (int i = 0; i < count; i++) {
    output_ptr[i] = MuLaw_Encode(float_to_int16(input_ptr[i]));
  }
}

__attribute__((export_name("decode_audio")))
void decode_audio(int8_t* input_ptr, float* output_ptr, int count) {
  for (int i = 0; i < count; i++) {
    output_ptr[i] = int16_to_float(MuLaw_Decode(input_ptr[i]));
  }
}

__attribute__((export_name("resample_f32")))
uint32_t resample_f32(const float *input, float *output, int inSampleRate, int outSampleRate, uint32_t inputSize) {
    uint32_t outputSize = (uint32_t) (inputSize * (double) outSampleRate / (double) inSampleRate);

    if (output == NULL)
        return outputSize;
    if (input == NULL)
        return 0;
    double stepDist = ((double) inSampleRate / (double) outSampleRate);
    const uint64_t fixedFraction = (1LL << 32);
    const double normFixed = (1.0 / (1LL << 32));
    uint64_t step = ((uint64_t) (stepDist * fixedFraction + 0.5));
    uint64_t curOffset = 0;

    for (uint32_t i = 0; i < outputSize - 1; i += 1) {
        uint64_t currentPos = curOffset >> 32;
        uint64_t nextPos = currentPos + 1;
        float current = input[currentPos];
        float next = input[nextPos];
        double frac = (curOffset & (fixedFraction - 1)) * normFixed;
        *output++ = (float) (current + (next - current) * frac);
        curOffset += step;
        uint64_t frameSkip = (curOffset >> 32);
        curOffset &= (fixedFraction - 1);
        input += frameSkip;
    }
    uint64_t lastPos = (curOffset >> 32);
    *output++ = input[lastPos];
    return outputSize;
}
