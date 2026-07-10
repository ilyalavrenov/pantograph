package callsite

//pantograph:f
func entry() {
	//pantograph:f note="async via goroutine"
	helper()
}

//pantograph:f
func helper() {}
