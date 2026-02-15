#if UNITY_EDITOR
using System;
using System.Linq;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

namespace WizardBeardStudio.Rgs.Editor
{
    public static class SampleSceneWiringEditor
    {
        [MenuItem("Tools/WizardBeard/RGS/Wire AuthAndBalance Sample Scene")]
        public static void WireAuthAndBalanceScene()
        {
            WireScene("AuthAndBalanceFlow", "WizardBeardStudio.Rgs.Samples.AuthAndBalanceSample");
        }

        [MenuItem("Tools/WizardBeard/RGS/Wire QuickStartSlot Sample Scene")]
        public static void WireQuickStartSlotScene()
        {
            WireScene("AuthAndBalanceFlow", "WizardBeardStudio.Rgs.Samples.QuickStartSlotSample");
            WireScene("QuickStartSlotFlow", "WizardBeardStudio.Rgs.Samples.QuickStartSlotSample");
        }

        private static void WireScene(string flowObjectName, string sampleTypeName)
        {
            var scene = EditorSceneManager.GetActiveScene();
            if (!scene.IsValid())
            {
                Debug.LogError("No active scene to wire.");
                return;
            }

            var bootstrapGO = GameObject.Find("RgsBootstrap") ?? new GameObject("RgsBootstrap");
            var flowGO = GameObject.Find(flowObjectName) ?? new GameObject(flowObjectName);

            var bootstrapType = FindType("WizardBeardStudio.Rgs.RgsClientBootstrap");
            if (bootstrapType == null)
            {
                Debug.LogError("Could not find type WizardBeardStudio.Rgs.RgsClientBootstrap. Ensure package runtime assembly compiled.");
                return;
            }

            var sampleType = FindType(sampleTypeName);
            if (sampleType == null)
            {
                Debug.LogError($"Could not find sample type {sampleTypeName}. Import package samples before wiring.");
                return;
            }

            var bootstrapComponent = EnsureComponent(bootstrapGO, bootstrapType);
            var sampleComponent = EnsureComponent(flowGO, sampleType);

            var bootstrapField = sampleType.GetField("bootstrap", System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance);
            if (bootstrapField != null)
            {
                bootstrapField.SetValue(sampleComponent, bootstrapComponent);
            }

            EditorUtility.SetDirty(bootstrapGO);
            EditorUtility.SetDirty(flowGO);
            EditorSceneManager.MarkSceneDirty(scene);
            Debug.Log($"Wired sample scene objects: {bootstrapGO.name}, {flowGO.name}");
        }

        private static Type? FindType(string fullName)
        {
            return AppDomain.CurrentDomain.GetAssemblies()
                .SelectMany(a => a.GetTypes())
                .FirstOrDefault(t => t.FullName == fullName);
        }

        private static Component EnsureComponent(GameObject go, Type type)
        {
            var existing = go.GetComponent(type);
            if (existing != null)
            {
                return existing;
            }
            return go.AddComponent(type);
        }
    }
}
#endif
