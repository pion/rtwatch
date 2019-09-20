#include "gst.h"

#include <gst/app/gstappsrc.h>

typedef struct SampleHandlerUserData {
  int pipelineId;
} SampleHandlerUserData;

GMainLoop *gstreamer_send_main_loop = NULL;
void gstreamer_send_start_mainloop(void) {
  gstreamer_send_main_loop = g_main_loop_new(NULL, FALSE);

  g_main_loop_run(gstreamer_send_main_loop);
}

static gboolean gstreamer_send_bus_call(GstBus *bus, GstMessage *msg, gpointer data) {
  GstElement *pipeline = GST_ELEMENT(data);

  switch (GST_MESSAGE_TYPE(msg)) {
  case GST_MESSAGE_EOS:
    if (!gst_element_seek (pipeline, 1.0, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT | GST_SEEK_FLAG_SKIP,
             GST_SEEK_TYPE_SET, 0,
             GST_SEEK_TYPE_NONE, GST_CLOCK_TIME_NONE)) {
        g_print ("EOS restart failed\n");
        exit(1);
    }
    break;

  case GST_MESSAGE_ERROR: {
    gchar *debug;
    GError *error;

    gst_message_parse_error(msg, &error, &debug);
    g_free(debug);

    g_printerr("GStreamer Error: %s\n", error->message);
    g_error_free(error);
    exit(1);
  }
  default:
    break;
  }

  return TRUE;
}

GstFlowReturn gstreamer_send_new_sample_handler(GstElement *object, gpointer user_data) {
  GstSample *sample = NULL;
  GstBuffer *buffer = NULL;
  gpointer copy = NULL;
  gsize copy_size = 0;
  int *isVideo = (int *) user_data;

  g_signal_emit_by_name (object, "pull-sample", &sample);
  if (sample) {
    buffer = gst_sample_get_buffer(sample);
    if (buffer) {
      gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
      goHandlePipelineBuffer(copy, copy_size, GST_BUFFER_DURATION(buffer), *isVideo);
    }
    gst_sample_unref (sample);
  }

  return GST_FLOW_OK;
}

GstElement *gstreamer_send_create_pipeline(char *pipeline) {
  gst_init(NULL, NULL);
  GError *error = NULL;
  return gst_parse_launch(pipeline, &error);
}

void gstreamer_send_start_pipeline(GstElement *pipeline) {
  GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));
  gst_bus_add_watch(bus, gstreamer_send_bus_call, pipeline);
  gst_object_unref(bus);

  GstElement *audio = gst_bin_get_by_name(GST_BIN(pipeline), "audio"),
             *video = gst_bin_get_by_name(GST_BIN(pipeline), "video");

  int *isAudio = malloc(sizeof(int)),
      *isVideo = malloc(sizeof(int));

  *isVideo = 1;
  *isAudio = 0;

  g_object_set(audio, "emit-signals", TRUE, NULL);
  g_signal_connect(audio, "new-sample", G_CALLBACK(gstreamer_send_new_sample_handler), isAudio);

  g_object_set(video, "emit-signals", TRUE, NULL);
  g_signal_connect(video, "new-sample", G_CALLBACK(gstreamer_send_new_sample_handler), isVideo);

  gstreamer_send_play_pipeline(pipeline);
}

void gstreamer_send_play_pipeline(GstElement *pipeline) {
  gst_element_set_state(pipeline, GST_STATE_PLAYING);
}

void gstreamer_send_pause_pipeline(GstElement *pipeline) {
  gst_element_set_state(pipeline, GST_STATE_PAUSED);
}

void gstreamer_send_seek(GstElement *pipeline, int64_t seek_pos) {
    if (!gst_element_seek (pipeline, 1.0, GST_FORMAT_TIME, GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT | GST_SEEK_FLAG_SKIP,
             GST_SEEK_TYPE_SET, seek_pos * GST_SECOND,
             GST_SEEK_TYPE_NONE, GST_CLOCK_TIME_NONE)) {
        g_print ("Seek failed!\n");
    }
}
