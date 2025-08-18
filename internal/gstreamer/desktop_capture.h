#ifndef DESKTOP_CAPTURE_H
#define DESKTOP_CAPTURE_H

#include <gst/gst.h>
#include <gst/app/gstappsink.h>

// Forward declaration of the Go function
extern void goHandleAppsinkSample(GstSample *sample);

static inline GstFlowReturn new_sample_handler(GstAppSink *sink, gpointer user_data) {
    GstSample *sample = gst_app_sink_pull_sample(sink);
    if (sample) {
        goHandleAppsinkSample(sample);
        gst_sample_unref(sample);
        return GST_FLOW_OK;
    }
    return GST_FLOW_ERROR;
}

static inline void connect_appsink_callback(GstElement *appsink) {
    g_signal_connect(appsink, "new-sample", G_CALLBACK(new_sample_handler), NULL);
}

#endif // DESKTOP_CAPTURE_H
