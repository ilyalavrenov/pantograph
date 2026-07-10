package render

func dumpNode(n *Node) string {
	lane := newD2Map()
	addNode(lane, n, testShapes)

	return lane.format()
}

func dumpEdge(from, to string, e Edge, useNode bool) string {
	root := newD2Map()
	addEdge(root, from, to, e, useNode)

	return root.format()
}
