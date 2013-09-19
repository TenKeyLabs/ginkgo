package ginkgo

import (
	"github.com/onsi/ginkgo/config"

	"math/rand"
	"regexp"
	"sort"
	"time"
)

type testingT interface {
	Fail()
}

type exampleCollection struct {
	t                                 testingT
	description                       string
	examples                          []*example
	exampleCountBeforeParallelization int
	reporters                         []Reporter
	startTime                         time.Time
	runningExample                    *example
	config                            config.GinkgoConfigType
}

func newExampleCollection(t testingT, description string, examples []*example, reporters []Reporter, config config.GinkgoConfigType) *exampleCollection {
	collection := &exampleCollection{
		t:           t,
		description: description,
		examples:    examples,
		reporters:   reporters,
		config:      config,
		exampleCountBeforeParallelization: len(examples),
	}

	r := rand.New(rand.NewSource(config.RandomSeed))
	if config.RandomizeAllSpecs {
		collection.shuffle(r)
	}

	if config.FocusString != "" {
		collection.applyRegExpFocus(regexp.MustCompile(config.FocusString))
	} else {
		collection.applyProgrammaticFocus()
	}

	if config.SkipMeasurements {
		collection.skipMeasurements()
	}

	if config.ParallelTotal > 1 {
		collection.trimForParallelization(config.ParallelTotal, config.ParallelNode)
	}

	return collection
}

func (collection *exampleCollection) applyProgrammaticFocus() {
	hasFocusedTests := false
	for _, example := range collection.examples {
		if example.focused {
			hasFocusedTests = true
			break
		}
	}

	if hasFocusedTests {
		for _, example := range collection.examples {
			if !example.focused {
				example.skip()
			}
		}
	}
}

func (collection *exampleCollection) applyRegExpFocus(focusFilter *regexp.Regexp) {
	for _, example := range collection.examples {
		if !focusFilter.Match([]byte(example.concatenatedString())) {
			example.skip()
		}
	}
}

func (collection *exampleCollection) trimForParallelization(parallelTotal int, parallelNode int) {
	startIndex, count := parallelizedIndexRange(len(collection.examples), parallelTotal, parallelNode)
	if count == 0 {
		collection.examples = make([]*example, 0)
	} else {
		collection.examples = collection.examples[startIndex : startIndex+count]
	}
}

func (collection *exampleCollection) skipMeasurements() {
	for _, example := range collection.examples {
		if example.subjectComponentType() == ExampleComponentTypeMeasure {
			example.skip()
		}
	}
}

func (collection *exampleCollection) shuffle(r *rand.Rand) {
	sort.Sort(collection)
	permutation := r.Perm(len(collection.examples))
	shuffledExamples := make([]*example, len(collection.examples))
	for i, j := range permutation {
		shuffledExamples[i] = collection.examples[j]
	}
	collection.examples = shuffledExamples
}

func (collection *exampleCollection) run() {
	collection.reportSuiteWillBegin()

	suiteFailed := false

	for _, example := range collection.examples {
		if !example.skippedOrPending() {
			collection.runningExample = example
			example.run()
			if example.failed() {
				suiteFailed = true
			}
		} else if example.pending() && collection.config.FailOnPending {
			suiteFailed = true
		}

		collection.reportExample(example)
	}

	collection.reportSuiteDidEnd()

	if suiteFailed {
		collection.t.Fail()
	}
}

func (collection *exampleCollection) fail(failure failureData) {
	if collection.runningExample != nil {
		collection.runningExample.fail(failure)
	}
}

func (collection *exampleCollection) reportSuiteWillBegin() {
	collection.startTime = time.Now()
	summary := collection.summary()
	for _, reporter := range collection.reporters {
		reporter.SpecSuiteWillBegin(collection.config, summary)
	}
}

func (collection *exampleCollection) reportExample(example *example) {
	summary := example.summary()
	for _, reporter := range collection.reporters {
		reporter.ExampleDidComplete(summary)
	}
}

func (collection *exampleCollection) reportSuiteDidEnd() {
	summary := collection.summary()
	summary.RunTime = time.Since(collection.startTime)
	for _, reporter := range collection.reporters {
		reporter.SpecSuiteDidEnd(summary)
	}
}

func (collection *exampleCollection) countExamplesSatisfying(filter func(ex *example) bool) (count int) {
	count = 0

	for _, example := range collection.examples {
		if filter(example) {
			count++
		}
	}

	return count
}

func (collection *exampleCollection) summary() *SuiteSummary {
	numberOfExamplesThatWillBeRun := collection.countExamplesSatisfying(func(ex *example) bool {
		return !ex.skippedOrPending()
	})

	numberOfPendingExamples := collection.countExamplesSatisfying(func(ex *example) bool {
		return ex.state == ExampleStatePending
	})

	numberOfSkippedExamples := collection.countExamplesSatisfying(func(ex *example) bool {
		return ex.state == ExampleStateSkipped
	})

	numberOfPassedExamples := collection.countExamplesSatisfying(func(ex *example) bool {
		return ex.state == ExampleStatePassed
	})

	numberOfFailedExamples := collection.countExamplesSatisfying(func(ex *example) bool {
		return ex.failed()
	})

	success := true

	if numberOfFailedExamples > 0 {
		success = false
	} else if numberOfPendingExamples > 0 && collection.config.FailOnPending {
		success = false
	}

	return &SuiteSummary{
		SuiteDescription: collection.description,
		SuiteSucceeded:   success,

		NumberOfExamplesBeforeParallelization: collection.exampleCountBeforeParallelization,
		NumberOfTotalExamples:                 len(collection.examples),
		NumberOfExamplesThatWillBeRun:         numberOfExamplesThatWillBeRun,
		NumberOfPendingExamples:               numberOfPendingExamples,
		NumberOfSkippedExamples:               numberOfSkippedExamples,
		NumberOfPassedExamples:                numberOfPassedExamples,
		NumberOfFailedExamples:                numberOfFailedExamples,
	}
}

//sort.Interface

func (collection *exampleCollection) Len() int {
	return len(collection.examples)
}

func (collection *exampleCollection) Less(i, j int) bool {
	return collection.examples[i].concatenatedString() < collection.examples[j].concatenatedString()
}

func (collection *exampleCollection) Swap(i, j int) {
	collection.examples[i], collection.examples[j] = collection.examples[j], collection.examples[i]
}
