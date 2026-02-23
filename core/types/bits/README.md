# bits

```go
import "github.com/altessa-s/go-atlas/core/types/bits"
```

Package `bits` provides generic bit manipulation functions for all integer types. Covers single-bit operations, range operations, counting, rotation, 
and masking. All functions are generic over `constraints.Integer`.

## Single-bit operations

| Function    | Description                          |
|-------------|--------------------------------------|
| `IsBitSet`  | Test whether bit at position is set  |
| `SetBit`    | Set bit to 1                         |
| `ClearBit`  | Set bit to 0                         |
| `ToggleBit` | Flip bit                             |
| `SwapBits`  | Exchange bits at two positions       |

## Range operations

| Function     | Description                                    |
|--------------|------------------------------------------------|
| `GetBits`    | Extract contiguous bit range, right-aligned    |
| `SetBits`    | Set contiguous range to 1                      |
| `ClearBits`  | Set contiguous range to 0                      |
| `ToggleBits` | Flip contiguous range                          |
| `TestBits`   | Check if all bits in range are set             |

## Counting and searching

| Function             | Description                                  |
|----------------------|----------------------------------------------|
| `CountSetBits`       | Population count (Hamming weight)            |
| `CountTrailingZeros` | Trailing zero count from LSB                 |
| `CountLeadingZeros`  | Leading zero count from MSB                  |
| `FindFirstSet`       | 1-based position of lowest set bit           |
| `FindLastSet`        | 1-based position of highest set bit          |
| `FindNextSet`        | Next set bit after a given position          |

## Rotation and transformation

| Function      | Description                                      |
|---------------|--------------------------------------------------|
| `RotateLeft`  | Circular left rotation                           |
| `RotateRight` | Circular right rotation                          |
| `ReverseBits` | Mirror all bits (LSB becomes MSB)                |
| `SignExtend`  | Sign-extend a narrow value to full width         |

## Masking

| Function      | Description                              |
|---------------|------------------------------------------|
| `CreateMask`  | Create bitmask with N consecutive 1-bits |
| `ApplyMask`   | Bitwise AND of value and mask            |

## Predicates

| Function       | Description                              |
|----------------|------------------------------------------|
| `IsPowerOfTwo` | True if exactly one bit is set           |
| `Parity`       | 1 if odd number of set bits, 0 otherwise |

## Constants

`BitsInByte`, `BitsInUint16`, `BitsInUint32`, `BitsInUint64` and corresponding `MaxBitPosition*` constants.
