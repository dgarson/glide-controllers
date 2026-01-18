// Package streamlinetest provides testing utilities for Streamline handlers.
//
// This package makes it easy to unit test handler implementations without
// requiring a real Kubernetes cluster or complex mock setups.
//
// # Basic Usage
//
// Create a fake context for testing your handler:
//
//	func TestMyHandler_Sync(t *testing.T) {
//	    obj := &v1.MyResource{
//	        ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
//	        Spec: v1.MyResourceSpec{Replicas: 3},
//	    }
//
//	    ctx := streamlinetest.NewFakeContext(t,
//	        streamlinetest.WithObject(obj),
//	    )
//
//	    handler := &MyHandler{}
//	    result, err := handler.Sync(context.Background(), obj, ctx)
//
//	    streamlinetest.AssertNoError(t, err)
//	    streamlinetest.AssertNoRequeue(t, result)
//	    streamlinetest.AssertConditionTrue(t, obj, "Ready")
//	}
//
// # Assertions
//
// The package provides various assertion helpers:
//
//	streamlinetest.AssertNoError(t, err)
//	streamlinetest.AssertNoRequeue(t, result)
//	streamlinetest.AssertRequeue(t, result)
//	streamlinetest.AssertRequeueAfter(t, result, 5*time.Minute)
//	streamlinetest.AssertConditionTrue(t, obj, "Ready")
//	streamlinetest.AssertConditionFalse(t, obj, "Ready")
//	streamlinetest.AssertPhase(t, obj, "Running")
//	streamlinetest.AssertEventRecorded(t, ctx, "Normal", "Synced")
//
// # Fake Client
//
// The package includes a configurable fake client:
//
//	client := streamlinetest.NewFakeClient(
//	    streamlinetest.WithExistingObjects(configMap, secret),
//	    streamlinetest.WithGetError(apierrors.NewNotFound(...)),
//	)
//
// # Test Handlers
//
// Create test handlers for engine testing:
//
//	handler := streamlinetest.NewFakeHandler[*v1.MyResource]().
//	    WithSync(func(ctx context.Context, obj *v1.MyResource, sCtx *streamline.Context) (streamline.Result, error) {
//	        obj.Status.Phase = "Running"
//	        return streamline.Stop(), nil
//	    })
package streamlinetest
