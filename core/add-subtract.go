package core
/* Implement addition and subtraction for SMPC
*/
import "math/big"
func SmpcAdd (share1 int64, share2 int64, prime *big.Int) (int64) {
    share1b := big.NewInt(share1)
    share2b := big.NewInt(share2)
    share1b.Add(share1b, share2b)
    share2b.Mod(share1b, prime) 
    return share2b.Int64() //Quite possibly the simplest way possible
}

func Add (share1 int64, share2 int64) (int64) {
    return SmpcAdd(share1, share2, LargePrime)
}

func SmpcSub (share1 int64, share2 int64, prime *big.Int) (int64) {
    share1b := big.NewInt(share1)
    share2b := big.NewInt(share2)
    share1b.Sub(share1b, share2b)
    share2b.Mod(share1b, prime)
    return share2b.Int64()
}

func Sub (share1 int64, share2 int64) (int64) {
    return SmpcSub(share1, share2, LargePrime)
}
