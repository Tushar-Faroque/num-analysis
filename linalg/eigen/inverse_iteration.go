package eigen

import (
	"errors"
	"math"
	"math/rand"
	"time"

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
	return InverseIterationPrec(m, maxIters, 0)
}

// InverseIterationPrec is like InverseIteration, but
// it stops making better approximations of each
// eigenvalue after a certain backwards error is
// achieved, where backwards error is defined as
// norm(Av-xv) where v is the approximate eigenvector
// and x is the approximate eigenvalue.
func InverseIterationPrec(m *linalg.Matrix, maxIters int,
	prec float64) ([]float64, []linalg.Vector, error) {
	if !m.Square() {
		panic("input matrix must be square")
	}
	iterator := inverseIterator{
		matrix:              m,
		remainingIterations: maxIters,
		eigenVectors:        make([]linalg.Vector, 0, m.Rows),
		eigenValues:         make([]float64, 0, m.Rows),
		precThreshold:       prec,
	}

	var err error
	for i := 0; i < m.Rows; i++ {
		if err = iterator.findNextVector(); err != nil {
			break
		}
	}

	return iterator.eigenValues, iterator.eigenVectors, err
}

// InverseIterationTime is like InverseIteration, but
// it uses the given timeout to determine how closely
// to approximate the eigenvalues and eigenvectors.
// The more time this routine is given, the more
// accurate the resulting eigenvalues and eigenvectors
// are likely to be.
func InverseIterationTime(m *linalg.Matrix, t time.Duration) ([]float64, []linalg.Vector) {
	if !m.Square() {
		panic("input matrix must be square")
	}
	iterator := inverseIterator{
		matrix:        m,
		eigenVectors:  make([]linalg.Vector, 0, m.Rows),
		eigenValues:   make([]float64, 0, m.Rows),
		useTime:       true,
		iterationTime: t / time.Duration(m.Rows*2),
	}

	for i := 0; i < m.Rows; i++ {
		iterator.findNextVector()
	}

	return iterator.eigenValues, iterator.eigenVectors
}

type inverseIterator struct {
	matrix              *linalg.Matrix
	remainingIterations int
	eigenVectors        []linalg.Vector
	eigenValues         []float64

	// precThreshold is non-zero if backErrorCriteria should
	// be used instead of oscillationCriteria.
	precThreshold float64

	useTime       bool
	iterationTime time.Duration
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
	epsilon := math.Nextafter(1, 2) - 1

	vec := i.randomStart()
	i.deleteProjections(vec)
	val := i.scaleFactor(vec)

	criteria := i.convergenceCriteria()
	criteria.Step(i.backError(val, vec), val, vec)

	cache := newLUCache()

	for i.remainingIterations > 0 || i.useTime {
		i.remainingIterations--
		lu := cache.get(val)
		if lu == nil {
			mat := i.shiftedMatrix(val)
			lu = ludecomp.Decompose(mat)
			cache.set(val, lu)
			if lu.PivotScale() < epsilon {
				return val, vec
			}
		}
		vec = lu.Solve(vec)
		normalizeMaxElement(vec)
		i.deleteProjections(vec)
		normalizeMaxElement(vec)
		val = i.scaleFactor(vec)

		criteria.Step(i.backError(val, vec), val, vec)
		if criteria.Converging() {
			return criteria.Best()
		}
	}

	return 0, nil
}

func (i *inverseIterator) powerIterate(val float64, vec linalg.Vector) (float64, linalg.Vector) {
	criteria := i.convergenceCriteria()
	criteria.Step(i.backError(val, vec), val, vec)

	for i.remainingIterations > 0 || i.useTime {
		i.remainingIterations--
		vec = i.matrix.Mul(linalg.NewMatrixColumn(vec)).Col(0)
		normalizeMaxElement(vec)
		i.deleteProjections(vec)
		normalizeMaxElement(vec)
		val = i.scaleFactor(vec)

		criteria.Step(i.backError(val, vec), val, vec)
		if criteria.Converging() {
			return criteria.Best()
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
		errorSum.Add(math.Pow(productVal-val*x, 2))
	}
	return math.Sqrt(errorSum.Sum())
}

func (i *inverseIterator) convergenceCriteria() convergenceCriteria {
	if i.precThreshold != 0 {
		return newBackErrorCriteria(i.precThreshold)
	} else if i.useTime {
		return newTimeCriteria(i.iterationTime)
	} else {
		return newOscillationCriteria(i.matrix)
	}
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