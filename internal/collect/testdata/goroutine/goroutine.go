package goroutine

//pantograph:g
func dispatcher() {
	go loop()
	sync()
}

//pantograph:g
func loop() {}

//pantograph:g
func sync() {}

//pantograph:g
func wrapped() {
	go func() {
		inner()
	}()
}

//pantograph:g
func inner() {}

//pantograph:g
func orMerge() {
	dup()
	go dup()
}

//pantograph:g
func dup() {}

//pantograph:g
func argDispatch() {
	go consume(produce())
}

//pantograph:g
func consume(int) {}

//pantograph:g
func produce() int { return 0 }
