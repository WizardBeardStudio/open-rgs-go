using UnityEditor;
using UnityEngine;

namespace WizardBeardStudio.Rgs.Editor
{
    [CustomEditor(typeof(RgsClientBootstrap))]
    public sealed class RgsClientBootstrapEditor : UnityEditor.Editor
    {
        public override void OnInspectorGUI()
        {
            DrawDefaultInspector();
            EditorGUILayout.Space();
            EditorGUILayout.HelpBox(
                "RGS Bootstrap initializes SDK services on Awake(). Configure endpoint, actor metadata defaults, and transport mode before Play.",
                MessageType.Info);
        }
    }
}
