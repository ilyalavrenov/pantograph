package derive

//pantograph:f
func funcA() {
	funcB()
	funcD()
	funcE()
}

//pantograph:f
func funcB() {}

//pantograph:f
func funcC() {
	funcD()
}

func funcD() {}

//pantograph:g
func funcE() {}

type iface interface {
	M()
}

type impl struct{}

//pantograph:f
func (impl) M() {}

//pantograph:f
func caller(i iface) {
	i.M()
}
