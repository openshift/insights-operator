package clusterconfig

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_RangeIsOverlapping(t *testing.T) {
	tests := []struct {
		name     string
		r1       uidRange
		r2       uidRange
		expected bool
	}{
		{
			name: "Same ranges",
			r1: uidRange{
				starting: 1000680000,
				length:   10000,
			},
			r2: uidRange{
				starting: 1000680000,
				length:   10000,
			},
			expected: true,
		},
		{
			name: "Different ranges",
			r1: uidRange{
				starting: 1000680000,
				length:   10000,
			},
			r2: uidRange{
				starting: 1000690000,
				length:   10000,
			},
			expected: false,
		},
		{
			name: "Overlapping ranges 1",
			r1: uidRange{
				starting: 1000680000,
				length:   10000,
			},
			r2: uidRange{
				starting: 1000689000,
				length:   10000,
			},
			expected: true,
		},
		{
			name: "Overlapping ranges 2",
			r1: uidRange{
				starting: 1000710000,
				length:   10000,
			},
			r2: uidRange{
				starting: 1000705000,
				length:   8000,
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := test.r1.IsOverlappingWith(test.r2)
			assert.Equal(t, test.expected, r)
		})
	}
}

func Test_GatherNamespacesWithOverlappingUIDs(t *testing.T) { //nolint: funlen
	tests := []struct {
		name           string
		namespaces     []*v1.Namespace
		expectedResult SetOfNamespaceSets
		errors         []error
	}{
		{
			name: "No overlapping namespaces",
			namespaces: []*v1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-1",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "10000/1000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-2",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "11000/1000",
						},
					},
				},
			},
			expectedResult: SetOfNamespaceSets(nil),
			errors:         []error(nil),
		},
		{
			name: "Overlapping namespaces and one wrong annotation value",
			namespaces: []*v1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-1",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "10000/2000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-2",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "not a range",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-3",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "11000/1000",
						},
					},
				},
			},
			expectedResult: SetOfNamespaceSets{
				NewSet(namespaceWithRange{
					name: "test-1",
					uidRange: uidRange{
						starting: 10000,
						length:   2000,
					},
				}, namespaceWithRange{
					name: "test-3",
					uidRange: uidRange{
						starting: 11000,
						length:   1000,
					},
				}),
			},
			errors: []error{fmt.Errorf("can't read uid range of the test-2 namespace")},
		},
		{
			name: "Some overlapping pairs",
			namespaces: []*v1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-1",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000697000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-2",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000690000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-3",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000700000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-5",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000800000/10000",
						},
					},
				},
			},
			expectedResult: SetOfNamespaceSets{
				NewSet(namespaceWithRange{
					name: "test-1",
					uidRange: uidRange{
						starting: 1000697000,
						length:   10000,
					},
				}, namespaceWithRange{
					name: "test-2",
					uidRange: uidRange{
						starting: 1000690000,
						length:   10000,
					},
				}),
				NewSet(namespaceWithRange{
					name: "test-1",
					uidRange: uidRange{
						starting: 1000697000,
						length:   10000,
					},
				}, namespaceWithRange{
					name: "test-3",
					uidRange: uidRange{
						starting: 1000700000,
						length:   10000,
					},
				}),
			},
			errors: []error(nil),
		},
		{
			name: "Three overlapping namespaces and some other sets",
			namespaces: []*v1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-1",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000670000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-2",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000695000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-3",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000690000/8000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-4",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000700000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-5",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000697000/2000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-6",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000740000/10000",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-7",
						Annotations: map[string]string{
							"openshift.io/sa.scc.uid-range": "1000735000/10000",
						},
					},
				},
			},
			expectedResult: SetOfNamespaceSets{
				NewSet(namespaceWithRange{
					name: "test-2",
					uidRange: uidRange{
						starting: 1000695000,
						length:   10000,
					},
				}, namespaceWithRange{
					name: "test-3",
					uidRange: uidRange{
						starting: 1000690000,
						length:   8000,
					},
				}, namespaceWithRange{
					name: "test-5",
					uidRange: uidRange{
						starting: 1000697000,
						length:   2000,
					},
				}),
				NewSet(namespaceWithRange{
					name: "test-2",
					uidRange: uidRange{
						starting: 1000695000,
						length:   10000,
					},
				}, namespaceWithRange{
					name: "test-4",
					uidRange: uidRange{
						starting: 1000700000,
						length:   10000,
					},
				}),
				NewSet(namespaceWithRange{
					name: "test-6",
					uidRange: uidRange{
						starting: 1000740000,
						length:   10000,
					},
				}, namespaceWithRange{
					name: "test-7",
					uidRange: uidRange{
						starting: 1000735000,
						length:   10000,
					},
				}),
			},
			errors: []error(nil),
		},
	}

	corev1I := kubefake.NewSimpleClientset().CoreV1()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// create all the testing namespaces
			for _, n := range test.namespaces {
				_, err := corev1I.Namespaces().Create(context.TODO(), n, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			recs, errs := gatherNamespacesWithOverlappingUIDs(context.Background(), corev1I)
			assert.EqualValues(t, test.errors, errs)
			assert.Len(t, recs, 1)
			assert.EqualValues(t, test.expectedResult, recs[0].Item)

			// delete all the testing namespaces
			for _, n := range test.namespaces {
				err := corev1I.Namespaces().Delete(context.TODO(), n.Name, metav1.DeleteOptions{})
				assert.NoError(t, err)
			}
		})
	}
}
