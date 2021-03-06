package main

import (
	"github.com/unixpickle/num-analysis/linalg"
	"github.com/unixpickle/num-analysis/linalg/cholesky"
)

type Transformation struct {
	Matrix     [4]float64
	TranslateX float64
	TranslateY float64
}

func ApproxTransformation(source, destination []Point) *Transformation {
	columnVectors := sourceColumnEquations(source)
	normalMatrix := linalg.NewMatrix(6, 6)
	for i, v1 := range columnVectors {
		for j, v2 := range columnVectors {
			normalMatrix.Set(i, j, v1.Dot(v2))
		}
	}

	results := make(linalg.Vector, len(destination)*2)
	for i, point := range destination {
		twoI := i * 2
		results[twoI] = point.X
		results[twoI+1] = point.Y
	}
	normalResult := make(linalg.Vector, 6)
	for i, vec := range columnVectors {
		normalResult[i] = vec.Dot(results)
	}

	solution := cholesky.Decompose(normalMatrix).Solve(normalResult)
	return &Transformation{
		Matrix:     [4]float64{solution[0], solution[1], solution[3], solution[4]},
		TranslateX: solution[2],
		TranslateY: solution[5],
	}
}

func (t *Transformation) Apply(p Point) Point {
	return Point{
		X: p.X*t.Matrix[0] + p.Y*t.Matrix[1] + t.TranslateX,
		Y: p.X*t.Matrix[2] + p.Y*t.Matrix[3] + t.TranslateY,
	}
}

func sourceColumnEquations(points []Point) [6]linalg.Vector {
	var columnVectors [6]linalg.Vector
	for i := range columnVectors {
		columnVectors[i] = make(linalg.Vector, len(points)*2)
	}
	for i, point := range points {
		twoI := i * 2
		columnVectors[0][twoI] = point.X
		columnVectors[1][twoI] = point.Y
		columnVectors[2][twoI] = 1
		columnVectors[3][twoI+1] = point.X
		columnVectors[4][twoI+1] = point.Y
		columnVectors[5][twoI+1] = 1
	}
	return columnVectors
}
