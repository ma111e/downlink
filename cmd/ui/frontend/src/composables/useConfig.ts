import {ref, type Ref} from 'vue';
import {GetConfig, SaveConfig} from "../../wailsjs/go/downlinkclient/DownlinkClient";
import {models} from "../../wailsjs/go/models.ts";
import ServerConfig = models.ServerConfig;

export function useConfig() {
    const config: Ref<ServerConfig | null> = ref(null);
    const loading = ref(false);
    const error = ref<string | null>(null);

    // Fetch the current configuration
    const fetchConfig = async () => {
        loading.value = true;
        error.value = null;

        try {
            const result = await GetConfig();
            config.value = result;
        } catch (err) {
            console.error('Failed to fetch models:', err);
            error.value = err instanceof Error ? err.message : 'Failed to fetch models';
        } finally {
            loading.value = false;
        }
    };

    // Save the configuration
    const saveConfig = async (newConfig: ServerConfig) => {
        loading.value = true;
        error.value = null;

        try {
            await SaveConfig(newConfig);
            // Reload the models to ensure UI is in sync with the saved models
            await fetchConfig();
        } catch (err) {
            console.error('Failed to save models:', err);
            error.value = err instanceof Error ? err.message : 'Failed to save models';
        } finally {
            loading.value = false;
        }
    };

    return {
        config,
        loading,
        error,
        fetchConfig,
        saveConfig
    };
}