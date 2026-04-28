import {defineStore} from 'pinia';
import {computed, ref} from 'vue';
import {
    GetAnalysisConfig,
    GetAllArticleAnalyses,
    GetAvailableModels,
    GetLLMProviders
} from "../../wailsjs/go/downlinkclient/DownlinkClient";
import {models} from "../../wailsjs/go/models.ts";
import {useToast} from "@/components/ui/toast";
import AnalysisConfig = models.AnalysisConfig;

export const useArticleAnalysisStore = defineStore('articleAnalysis', () => {
    const {toast} = useToast();

    // Analysis state
    const isLoadingAnalyses = ref(false);
    const analysisResult = ref<models.ArticleAnalysis | null>(null);
    const allAnalyses = ref<models.ArticleAnalysis[]>([]);
    const selectedAnalysisId = ref<string | null>(null);

    // Provider and model selection state
    const providers = ref<models.ProviderConfig[]>([]);
    const availableModels = ref<models.ModelInfo[]>([]);
    const isLoadingProviders = ref(false);
    const isLoadingModels = ref(false);
    // Used by AnalysisDialog for on-demand per-article analysis
    const selectedProviderType = ref('');
    const selectedModelName = ref('');
    // Used by Analysis settings — links to a named provider config
    const selectedProvider = ref('');

    const analysisConfig = ref<AnalysisConfig | null>(null);
    const persona = ref('');

    // Computed property to check if there are analyses
    const hasAnalyses = computed(() => allAnalyses.value.length > 0);

    // Navigation between runs (allAnalyses is sorted newest-first)
    const currentIndex = computed(() => {
        if (!selectedAnalysisId.value) return -1;
        return allAnalyses.value.findIndex((a: models.ArticleAnalysis) => a.id === selectedAnalysisId.value);
    });

    // "prev" = newer run (lower index), "next" = older run (higher index)
    const canGoPrev = computed(() => currentIndex.value > 0);
    const canGoNext = computed(() =>
        currentIndex.value >= 0 && currentIndex.value < allAnalyses.value.length - 1
    );

    // Get unique provider types that are enabled
    const uniqueProviderTypes = computed(() => {
        const types = new Set<string>();
        providers.value
            .filter((p: models.ProviderConfig) => p.enabled)
            .forEach((p: models.ProviderConfig) => types.add(p.provider_type));
        return Array.from(types);
    });

    // Format provider type for display
    const formatProviderType = (type: string) => {
        return type.charAt(0).toUpperCase() + type.slice(1);
    };

    // Fetch analyses for an article
    const fetchArticleAnalyses = async (articleId: string) => {
        console.log("Fetching analyses for article:", articleId);
        if (!articleId) return;

        try {
            isLoadingAnalyses.value = true;
            const analyses = await GetAllArticleAnalyses(articleId);
            console.log("Fetched analyses:", analyses);
            allAnalyses.value = analyses || [];

            console.log("allAnalyses.value", allAnalyses.value);
            // If there are analyses, select the latest one
            if (allAnalyses.value.length > 0) {

                // Sort by creation date (newest first)
                allAnalyses.value.sort((a: models.ArticleAnalysis, b: models.ArticleAnalysis) =>
                    new Date(String(b.created_at)).getTime() - new Date(String(a.created_at)).getTime()
                );
                selectedAnalysisId.value = allAnalyses.value[0].id;
                analysisResult.value = allAnalyses.value[0];
            } else {
                selectedAnalysisId.value = null;
                analysisResult.value = null;
            }
        } catch (error) {
            console.error("Error fetching analyses:", error);
            toast({
                title: "Error",
                description: "Failed to load analyses",
                variant: "destructive",
                duration: 3000,
            });
        } finally {
            isLoadingAnalyses.value = false;
        }
    };

    const goToPrevAnalysis = async (articleId: string) => {
        if (canGoPrev.value) {
            const prevId = allAnalyses.value[currentIndex.value - 1].id;
            await selectAnalysis(articleId, prevId);
        }
    };

    const goToNextAnalysis = async (articleId: string) => {
        if (canGoNext.value) {
            const nextId = allAnalyses.value[currentIndex.value + 1].id;
            await selectAnalysis(articleId, nextId);
        }
    };

    // Select a specific analysis
    const selectAnalysis = async (articleId: string, analysisId: string) => {
        // Always fetch the latest analyses for the article to ensure we have the correct data
        await fetchArticleAnalyses(articleId);

        const analysis = allAnalyses.value.find((a: models.ArticleAnalysis) => a.id === analysisId);
        if (analysis) {
            selectedAnalysisId.value = analysisId;
            analysisResult.value = analysis;
        }
    };

    // Load LLM providers for analysis dialog
    const loadLLMProviders = async () => {
        try {
            isLoadingProviders.value = true;
            const result = await GetLLMProviders();
            providers.value = result || [];

            // If no enabled providers found
            if (!uniqueProviderTypes.value.length) {
                toast({
                    title: "No LLM Providers",
                    description: "No enabled LLM providers found. Please enable a provider in settings.",
                    variant: "destructive",
                    duration: 5000,
                });
            }


        } catch (error) {
            console.error("Error loading LLM providers:", error);
            toast({
                title: "Error",
                description: "Failed to load LLM providers",
                variant: "destructive",
                duration: 3000
            });
        } finally {
            isLoadingProviders.value = false;
        }
    };

    // Fetch models for a specific provider type
    const fetchModelsForProvider = async (providerType: string) => {
        if (!providerType) return;

        try {
            isLoadingModels.value = true;
            selectedModelName.value = ''; // Reset selected model
            const models = await GetAvailableModels();

            // Filter models based on the provider type
            const filteredModels = models ? models.models.filter((model: models.ModelInfo) => model.provider_type === providerType) : [];
            availableModels.value = filteredModels;

            // Automatically select the first model if available
            if (availableModels.value.length > 0) {
                selectedModelName.value = availableModels.value[0].name;
            }
        } catch (error) {
            console.error(`Error fetching models for ${providerType}:`, error);
            toast({
                title: "Error",
                description: `Failed to fetch models for ${formatProviderType(providerType)}`,
                variant: "destructive",
                duration: 3000
            });
        } finally {
            isLoadingModels.value = false;
        }
    };

    const loadAnalysisConfig = async () => {
        try {
            analysisConfig.value = await GetAnalysisConfig();

            if (analysisConfig.value) {
                selectedProvider.value = analysisConfig.value.provider || '';
                persona.value = analysisConfig.value.persona || '';
            }
        } catch (error) {
            console.error('Failed to load analysis configuration:', error);
            toast({
                title: "Error",
                description: `Failed to load analysis configuration: ${error}`,
                variant: "destructive"
            });
        }
    };

    // Reset all analysis state
    const resetAnalysisState = () => {
        isLoadingAnalyses.value = false;
        analysisResult.value = null;
        allAnalyses.value = [];
        selectedAnalysisId.value = null;
    };

    return {
        // State
        isLoadingAnalyses,
        analysisResult,
        allAnalyses,
        selectedAnalysisId,
        providers,
        availableModels,
        isLoadingProviders,
        isLoadingModels,
        selectedProviderType,
        selectedModelName,
        selectedProvider,
        analysisConfig,
        persona,

        // Computed
        hasAnalyses,
        uniqueProviderTypes,
        currentIndex,
        canGoPrev,
        canGoNext,

        // Methods
        loadAnalysisConfig,
        fetchArticleAnalyses,
        selectAnalysis,
        goToPrevAnalysis,
        goToNextAnalysis,
        loadLLMProviders,
        fetchModelsForProvider,
        resetAnalysisState,
        formatProviderType
    };
});
