package eigen

import (
	"errors"
	"math"
	"math/rand"

	"github.com/unixpickle/num-analysis/kahan"
	"github.com/unixpickle/num-analysis/linalg"
	"github.com/unixpickle/num-analysis/linalg/ludecomp"
)

var ErrMaxSteps = errors.New("maximum steps exceeded")

// InverseIteration uses inverse iteration to approximate
// the eigenvalues and eigenvectors of a symmetric matrix m.
//
// The maxIters argument acts as a sort of "timeout".
// If the algorithm spends more than maxIters iterations
// looking for eigenvectors, then ErrMaxSteps is returned
// along with the eigenvalues and eigenvectors which were
// already found.
func InverseIteration(m *linalg.Matrix, maxIters int) ([]float64, []linalg.Vector, error) {
	if !m.Square() {
		panic("input matrix must be square")
	}
	iterator := inverseIterator{
		matrix:              m,
		remainingIterations: maxIters,
		eigenVectors:        make([]linalg.Vector, 0, m.Rows),
		eigenValues:         make([]float64, 0, m.Rows),
	}

	var err error
	for i := 0; i < m.Rows; i++ {
		if err = iterator.findNextVector(); err != nil {
			break
		}
	}

	return iterator.eigenValues, iterator.eigenVectors, err
}

type inverseIterator struct {
	matrix              *linalg.Matrix
	remainingIterations int
	eigenVectors        []linalg.Vector
	eigenValues         []float64
}

func (i *inverseIterator) findNextVector() error {
	val, vec := i.inverseIterate()
	if vec == nil {
		return ErrMaxSteps
	}
	val, vec = i.powerIterate(val, vec)
	if vec == nil {
		return ErrMaxSteps
	}
	normalizeTwoNorm(vec)
	i.eigenVectors = append(i.eigenVectors, vec)
	i.eigenValues = append(i.eigenValues, val)
	return nil
}

func (i *inverseIterator) inverseIterate() (float64, linalg.Vector) {
	// Once the pivots differ by sqrt(epsilon), we may lose
	// half of our double's precision when computing A^-1*x.
	// This seems like a logical place to stop trying to
	// find a nearer approximation.
	pivotThreshold := math.Sqrt(math.Nextafter(1, 2) - 1)

	vec := i.randomStart()
	i.deleteProjections(vec)
	val := i.scaleFactor(vec)

	for i.remainingIterations > 0 {
		i.remainingIterations--
		mat := i.shiftedMatrix(val)
		lu := ludecomp.Decompose(mat)
		if lu.PivotScale() < pivotThreshold {
			return val, vec
		}
		vec = lu.Solve(vec)
		i.deleteProjections(vec)
		normalizeMaxElement(vec)
		val = i.scaleFactor(vec)
	}
	return 0, nil
}

func (i *inverseIterator) powerIterate(val float64, vec linalg.Vector) (float64, linalg.Vector) {
	var lastError float64

	for i.remainingIterations > 0 {
		i.remainingIterations--
		vec = i.matrix.Mul(linalg.NewMatrixColumn(vec)).Col(0)
		normalizeMaxElement(vec)
		i.deleteProjections(vec)
		val = i.scaleFactor(vec)
		backError := i.backError(val, vec)
		if backError == 0 {
			return val, vec
		} else if lastError == 0 {
			lastError = backError
		} else {
			if backError >= lastError {
				return val, vec
			}
			lastError = backError
		}
	}

	return 0, nil
}

func (i *inverseIterator) deleteProjections(vec linalg.Vector) {
	for _, eigVec := range i.eigenVectors {
		projMag := eigVec.Dot(vec)
		for i, x := range eigVec {
			vec[i] -= projMag * x
		}
	}
}

func (i *inverseIterator) randomStart() linalg.Vector {
	res := make(linalg.Vector, i.matrix.Rows)
	for i := range res {
		res[i] = rand.Float64()*2 - 1
	}
	return res
}

func (i *inverseIterator) scaleFactor(v linalg.Vector) float64 {
	colVec := linalg.NewMatrixColumn(v)
	return v.Dot(i.matrix.Mul(colVec).Col(0)) / v.Dot(v)
}

func (i *inverseIterator) shiftedMatrix(s float64) *linalg.Matrix {
	mat := i.matrix.Copy()
	for j := 0; j < mat.Rows; j++ {
		mat.Set(j, j, mat.Get(j, j)-s)
	}
	return mat
}

func (i *inverseIterator) backError(val float64, vec linalg.Vector) float64 {
	multiplied := i.matrix.Mul(linalg.NewMatrixColumn(vec))
	errorSum := kahan.NewSummer64()
	for i, x := range vec {
		productVal := multiplied.Get(i, 0)
		errorSum.Add(math.Abs(productVal - val*x))
	}
	return errorSum.Sum()
}

// normalizeMaxElement normalizes the given vector using
// the infinity norm (i.e. the norm which returns the
// maximum component of the vector).
func normalizeMaxElement(v linalg.Vector) {
	var mag float64
	for _, x := range v {
		mag = math.Max(mag, math.Abs(x))
	}
	if mag == 0 {
		for i := range v {
			v[i] = 1
		}
	} else {
		v.Scale(1 / mag)
	}
}

// normalizeTwoNorm normalizes the given vector using
// the standard two-norm (a.k.a. the Euclidean norm).
func normalizeTwoNorm(v linalg.Vector) {
	v.Scale(1 / math.Sqrt(v.Dot(v)))
}
