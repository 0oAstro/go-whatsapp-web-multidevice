import FormRecipient from "./generic/FormRecipient.js";

export default {
  name: "SendVideo",
  components: {
    FormRecipient,
  },
  // define props
  props: {
    maxVideoSize: {
      type: String,
      required: true,
    },
  },
  data() {
    return {
      caption: "",
      view_once: false,
      compress: false,
      type: window.TYPEUSER,
      phone: "",
      loading: false,
      video_url: "",
    };
  },
  computed: {
    phone_id() {
      return this.phone + this.type;
    },
  },
  methods: {
    openModal() {
      $("#modalSendVideo")
        .modal({
          onApprove: function () {
            return false;
          },
        })
        .modal("show");
    },
    async handleSubmit() {
      try {
        if (!this.video_url && !$("#file_video")[0].files[0]) {
          throw new Error(
            "Please provide either a video URL or upload a video file."
          );
        }
        let response = await this.submitApi();
        showSuccessInfo(response);
        $("#modalSendVideo").modal("hide");
      } catch (err) {
        showErrorInfo(err);
      }
    },
    async submitApi() {
      this.loading = true;
      try {
        let payload = new FormData();
        payload.append("phone", this.phone_id);
        payload.append("caption", this.caption);
        payload.append("view_once", this.view_once);
        payload.append("compress", this.compress);

        if (this.video_url) {
          payload.append("video_url", this.video_url);
        } else {
          payload.append("video", $("#file_video")[0].files[0]);
        }

        let response = await window.http.post(`/send/video`, payload);
        this.handleReset();
        return response.data.message;
      } catch (error) {
        if (error.response) {
          throw new Error(error.response.data.message);
        }
        throw new Error(error.message);
      } finally {
        this.loading = false;
      }
    },
    handleReset() {
      this.caption = "";
      this.view_once = false;
      this.compress = false;
      this.phone = "";
      this.type = window.TYPEUSER;
      this.video_url = "";
      $("#file_video").val("");
    },
  },
  template: `
    <div class="blue card" @click="openModal()" style="cursor: pointer">
        <div class="content">
            <a class="ui blue right ribbon label">Send</a>
            <div class="header">Send Video</div>
            <div class="description">
                Send video
                <div class="ui blue horizontal label">mp4</div>
                up to
                <div class="ui blue horizontal label">{{ maxVideoSize }}</div>
            </div>
        </div>
    </div>
    
    <!--  Modal SendVideo  -->
    <div class="ui small modal" id="modalSendVideo">
        <i class="close icon"></i>
        <div class="header">
            Send Video
        </div>
        <div class="content">
            <form class="ui form">
                <FormRecipient v-model:type="type" v-model:phone="phone"/>
                
                <div class="field">
                    <label>Caption</label>
                    <textarea v-model="caption" placeholder="Type some caption (optional)..."
                              aria-label="caption"></textarea>
                </div>
                <div class="field">
                    <label>View Once</label>
                    <div class="ui toggle checkbox">
                        <input type="checkbox" aria-label="view once" v-model="view_once">
                        <label>Check for enable one time view</label>
                    </div>
                </div>
                <div class="field">
                    <label>Compress</label>
                    <div class="ui toggle checkbox">
                        <input type="checkbox" aria-label="compress" v-model="compress">
                        <label>Check for compressing video to smaller size</label>
                    </div>
                </div>
                <div class="field">
                    <label>Video URL</label>
                    <input v-model="video_url" type="text" placeholder="http://example.com/video.mp4" aria-label="video url">
                </div>
                <div class="field" style="padding-bottom: 30px">
                    <label>Video</label>
                    <input type="file" style="display: none" accept="video/*" id="file_video">
                    <label for="file_video" class="ui positive medium green left floated button" style="color: white">
                        <i class="ui upload icon"></i>
                        Upload video
                    </label>
                </div>
            </form>
        </div>
        <div class="actions">
            <div class="ui approve positive right labeled icon button" :class="{'loading': this.loading}"
                 @click="handleSubmit">
                Send
                <i class="send icon"></i>
            </div>
        </div>
    </div>
    `,
};
