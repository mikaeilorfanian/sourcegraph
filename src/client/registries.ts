import { ConfigurationCascade } from '../protocol'
import { ObservableEnvironment } from './environment'
import { Extension } from './extension'
import { CommandRegistry } from './providers/command'
import { ContributionRegistry } from './providers/contribution'
import { TextDocumentDecorationProviderRegistry } from './providers/decoration'
import { TextDocumentHoverProviderRegistry } from './providers/hover'
import { TextDocumentLocationProviderRegistry, TextDocumentReferencesProviderRegistry } from './providers/location'

/**
 * Registries is a container for all provider registries.
 *
 * @template X extension type
 * @template C configuration cascade type
 */
export class Registries<X extends Extension, C extends ConfigurationCascade> {
    constructor(private environment: ObservableEnvironment<X, C>) {}

    public readonly commands = new CommandRegistry()
    public readonly contribution = new ContributionRegistry(this.environment.environment)
    public readonly textDocumentDefinition = new TextDocumentLocationProviderRegistry()
    public readonly textDocumentImplementation = new TextDocumentLocationProviderRegistry()
    public readonly textDocumentReferences = new TextDocumentReferencesProviderRegistry()
    public readonly textDocumentTypeDefinition = new TextDocumentLocationProviderRegistry()
    public readonly textDocumentHover = new TextDocumentHoverProviderRegistry()
    public readonly textDocumentDecoration = new TextDocumentDecorationProviderRegistry()
}
