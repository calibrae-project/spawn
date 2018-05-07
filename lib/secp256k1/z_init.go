package secp256k1

/*
import (
	"os"
	"fmt"
	"time"
)


var (
	preG, preG128 []XY
	prec [64][16]XY
	fin XY
)


func ecmult_start() {
	return
	sta := time.Now()

	g := TheCurve.G

	// calculate 2^128*generator
	var g_128j XYZ
	g_128j.SetXY(&g)

	for i := 0; i < 128; i++ {
		g_128j.Double(&g_128j)
	}

	var g_128 XY
	g_128.SetXYZ(&g_128j)

    // precompute the tables with odd multiples
	preG = g.precomp(WindowG)
	preG128 = g_128.precomp(WindowG)

	// compute prec and fin
	var gg XYZ
	gg.SetXY(&g)
	ad := g
	var fn XYZ
	fn.Infinity = true
	for j:=0; j<64; j++ {
		prec[j][0].SetXYZ(&gg)
		fn.Add(&fn, &gg)
		for i:=1; i<16; i++ {
			gg.AddXY(&gg, &ad)
			prec[j][i].SetXYZ(&gg)
		}
		ad = prec[j][15]
	}
	fin.SetXYZ(&fn)
	fin.Neg(&fin)

	if false {
		f, _ := os.Create("z_prec.go")
		fmt.Fprintln(f, "package secp256k1\n\nvar prec = [64][16]XY {")
		for j:=0; j<64; j++ {
			fmt.Fprintln(f, " {")
			for i:=0; i<16; i++ {
				fmt.Fprintln(f, "{X:" + fe2str(&prec[j][i].X) + ", Y:" + fe2str(&prec[j][i].Y) + "},")
			}
			fmt.Fprintln(f, "},")
		}
		fmt.Fprintln(f, "}")
		f.Close()
	}

	if false {
		f, _ := os.Create("z_pre_g.go")
		fmt.Fprintln(f, "package secp256k1\n\nvar preG = []XY {")
		for i := range preG {
			fmt.Fprintln(f, "{X:" + fe2str(&preG[i].X) + ", Y:" + fe2str(&preG[i].Y) + "},")
		}
		fmt.Fprintln(f, "}")
		f.Close()
	}

	if false {
		f, _ := os.Create("z_pre_g_128.go")
		fmt.Fprintln(f, "package secp256k1\n\nvar preG128 = []XY {")
		for i := range preG128 {
			fmt.Fprintln(f, "{X:" + fe2str(&preG128[i].X) + ", Y:" + fe2str(&preG128[i].Y) + "},")
		}
		fmt.Fprintln(f, "}")
		f.Close()
	}

	if false {
		f, _ := os.Create("z_fin.go")
		fmt.Fprintln(f, "package secp256k1\n\nvar fim = XY {")
		fmt.Fprintln(f, "X:" + fe2str(&fin.X) + ", Y:" + fe2str(&fin.Y) + ",")
		fmt.Fprintln(f, "}")
		f.Close()
	}

	println("start done in", time.Now().Sub(sta).String())
}


func fe2str(f *Field) (s string) {
	s = fmt.Sprintf("Field{[10]uint32{0x%08x", f.n[0])
	for i:=1; i<len(f.n); i++ {
		s += fmt.Sprintf(", 0x%08x", f.n[i])
	}
	s += "}}"
	return
}


*/
