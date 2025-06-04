#include "textflag.h"

// func simdVWAPCalc(samples []sample, size int) (sum, sumSq, volSum float64)
TEXT ·simdVWAPCalc(SB), NOSPLIT, $0-64
    MOVQ samples_base+0(FP), SI    // SI = &samples[0]
    MOVQ samples_len+8(FP), CX     // CX = len(samples)
    MOVQ size+24(FP), DX           // DX = size
    XORPS X0, X0                   // X0 = sum = 0
    XORPS X1, X1                   // X1 = sumSq = 0
    XORPS X2, X2                   // X2 = volSum = 0

    // Process 4 samples at a time using AVX2
    SHRQ $2, DX                    // DX = size/4
    JZ scalar                      // If size < 4, use scalar code

vector:
    // Load 4 samples
    VMOVUPS (SI), Y3              // Y3 = [p0, v0, p1, v1, p2, v2, p3, v3]
    ADDQ $32, SI                  // SI += 32 (4 samples * 8 bytes)

    // Extract prices and volumes
    VEXTRACTF128 $1, Y3, X4       // X4 = [p2, v2, p3, v3]
    VSHUFPD $0x5, X3, X5, X5      // X5 = [v0, v1, v2, v3]
    VSHUFPD $0x0, X3, X6, X6      // X6 = [p0, p1, p2, p3]

    // Calculate p*v and p*p*v
    VMULPD X5, X6, X7             // X7 = [p0*v0, p1*v1, p2*v2, p3*v3]
    VMULPD X7, X6, X8             // X8 = [p0*p0*v0, p1*p1*v1, p2*p2*v2, p3*p3*v3]

    // Accumulate results
    VADDPD X7, X0, X0             // sum += p*v
    VADDPD X8, X1, X1             // sumSq += p*p*v
    VADDPD X5, X2, X2             // volSum += v

    DECQ DX
    JNZ vector

    // Horizontal sum of XMM registers
    VEXTRACTF128 $1, Y0, X3
    VADDPD X3, X0, X0
    VEXTRACTF128 $1, Y1, X3
    VADDPD X3, X1, X1
    VEXTRACTF128 $1, Y2, X3
    VADDPD X3, X2, X2

scalar:
    // Handle remaining samples
    MOVQ samples_len+8(FP), CX
    ANDQ $3, CX                    // CX = size % 4
    JZ done

scalar_loop:
    MOVSD (SI), X3                // X3 = price
    MOVSD 8(SI), X4               // X4 = volume
    ADDSD X4, X2                   // volSum += volume
    MULSD X4, X3                   // X3 = price * volume
    ADDSD X3, X0                   // sum += price * volume
    MULSD (SI), X3                 // X3 = price * price * volume
    ADDSD X3, X1                   // sumSq += price * price * volume
    ADDQ $16, SI                   // SI += 16 (2 float64s)
    DECQ CX
    JNZ scalar_loop

done:
    // Store results
    MOVSD X0, sum+32(FP)
    MOVSD X1, sumSq+40(FP)
    MOVSD X2, volSum+48(FP)
    RET 