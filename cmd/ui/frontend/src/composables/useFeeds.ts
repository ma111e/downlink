import { ref } from 'vue';
import { ListFeeds } from "../../wailsjs/go/downlinkclient/DownlinkClient";
import { models } from "../../wailsjs/go/models.ts";
import Feed = models.Feed;

const feeds = ref<Feed[]>([]);
const titleById = new Map<string, string>();
const loading = ref(false);
const error = ref('');

let inFlight: Promise<void> | null = null;

const fetchFeeds = async () => {
  if (inFlight) return inFlight;
  inFlight = (async () => {
    try {
      loading.value = true;
      const list = await ListFeeds();
      feeds.value = list;
      titleById.clear();
      for (const f of list) titleById.set(f.id, f.title);
    } catch (err) {
      error.value = 'Failed to load feeds';
      console.error(err);
    } finally {
      loading.value = false;
    }
  })();
  try {
    await inFlight;
  } finally {
    inFlight = null;
  }
};

const getFeedTitleSync = (feedId: string): string => {
  return titleById.get(feedId) ?? 'Unknown Feed';
};

const getFeedTitle = async (feedId: string): Promise<string> => {
  if (titleById.size === 0) await fetchFeeds();
  return getFeedTitleSync(feedId);
};

export function useFeeds() {
  return {
    feeds,
    loading,
    error,
    fetchFeeds,
    getFeedTitle,
    getFeedTitleSync,
  };
}
